package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type ClaudeRecord struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Message   *ClaudeMessage `json:"message"`
}

type ClaudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type SessionSummary struct {
	SourceName        string
	FilePath          string
	UserMessages      []string
	AssistantMessages []string
	ToolsUsed         map[string]bool
	CommitHash        string
	CommitSubject     string
	CommitAuthor      string
	CommitTime        string
	CommitSnippet     string
}

type SessionSource struct {
	Name           string
	Label          string
	CandidateRoots []string
	Parser         func(string, string) (SessionSummary, error)
	Git            bool
}

type SourceStatus struct {
	Name  string
	Label string
	State string
	Count int
	Error string
}

const maxCommitSnippetLength = 4000

var runGitLog = func(since time.Time) (string, error) {
	cmd := exec.Command(
		"git",
		"log",
		"--since", since.Format(time.RFC3339),
		"--format=%h%x1f%an%x1f%ad%x1f%s",
		"--date=iso-strict",
	)
	output, err := cmd.Output()
	return string(output), err
}

var runGitShow = func(hash string) (string, error) {
	cmd := exec.Command(
		"git",
		"show",
		"--stat",
		"--patch",
		"--find-renames",
		"--format=",
		"--no-ext-diff",
		"--no-color",
		"--unified=3",
		hash,
	)
	output, err := cmd.Output()
	return string(output), err
}

func main() {
	var vaultPath string

	rootCmd := &cobra.Command{
		Use:   "log-work",
		Short: "Ingest today's coding agent sessions and git commits into a local work log",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogWork(vaultPath)
		},
	}
	rootCmd.Flags().StringVar(&vaultPath, "vault", "", "Obsidian vault path to write the work log to; overrides OBSIDIAN_VAULT")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func runLogWork(vaultPath string) error {
	today := time.Now().Format("2006-01-02")

	summaries, counts, statuses := collectTodaySummaries(defaultSources(), func(msg string) {
		fmt.Println(msg)
	})

	fmt.Println(formatStatusBar(statuses))

	if len(summaries) == 0 {
		fmt.Println("No coding agent sessions or git commits found for today.")
		return nil
	}

	outputFile := resolveOutputFile(today, vaultPath)
	if err := writeMarkdownLog(outputFile, today, summaries, statuses); err != nil {
		return err
	}

	fmt.Printf("Found %s from today.\n\n", formatCounts(counts))
	fmt.Println("Work log written to:")
	fmt.Println(outputFile)

	return nil
}

func resolveOutputFile(date string, vaultPath string) string {
	fileName := fmt.Sprintf("work-log-%s.md", date)
	vault := strings.TrimSpace(vaultPath)
	if vault == "" {
		vault = strings.TrimSpace(os.Getenv("OBSIDIAN_VAULT"))
	}
	if vault == "" {
		return fileName
	}
	return filepath.Join(vault, fileName)
}

func defaultSources() []SessionSource {
	homeDir, _ := os.UserHomeDir()
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")

	return []SessionSource{
		{
			Name:           "Claude Code",
			Label:          "sessions",
			CandidateRoots: compactPaths(filepath.Join(homeDir, ".claude", "projects")),
			Parser:         parseClaudeSessionFile,
		},
		{
			Name:  "Codex",
			Label: "sessions",
			CandidateRoots: compactPaths(
				filepath.Join(homeDir, ".codex"),
				joinIfBase(localAppData, "codex"),
				joinIfBase(appData, "codex"),
			),
			Parser: parseGenericSessionFile,
		},
		{
			Name:  "Pi",
			Label: "sessions",
			CandidateRoots: compactPaths(
				filepath.Join(homeDir, ".pi"),
				joinIfBase(localAppData, "pi"),
				joinIfBase(appData, "pi"),
			),
			Parser: parseGenericSessionFile,
		},
		{
			Name:  "Git Commits",
			Label: "commits",
			Git:   true,
		},
	}
}

func joinIfBase(base string, elem string) string {
	if strings.TrimSpace(base) == "" {
		return ""
	}
	return filepath.Join(base, elem)
}

func compactPaths(paths ...string) []string {
	var compact []string
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			compact = append(compact, path)
		}
	}
	return compact
}

func existingCandidateRoots(candidates []string) []string {
	var roots []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == "." {
			continue
		}

		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			roots = append(roots, candidate)
		}
	}
	return roots
}

func collectTodaySummaries(sources []SessionSource, warn func(string)) ([]SessionSummary, map[string]int, []SourceStatus) {
	var summaries []SessionSummary
	var statuses []SourceStatus
	counts := map[string]int{}
	now := time.Now()

	for _, source := range sources {
		if source.Git {
			gitSummaries, status := collectGitCommits(now)
			if status.State == "error" {
				warn(fmt.Sprintf("Warning: could not read %s: %s", source.Name, status.Error))
			}
			summaries = append(summaries, gitSummaries...)
			counts[source.Name] = len(gitSummaries)
			statuses = append(statuses, status)
			continue
		}

		status := SourceStatus{Name: source.Name, Label: source.Label, State: "none"}
		roots := existingCandidateRoots(source.CandidateRoots)
		if len(roots) == 0 {
			status.State = "missing"
			warn(fmt.Sprintf("Warning: %s sessions directory not found in known locations.", source.Name))
			statuses = append(statuses, status)
			continue
		}

		for _, root := range roots {
			files, err := findTodaySessionFiles(root)
			if err != nil {
				status.State = "error"
				status.Error = err.Error()
				warn(fmt.Sprintf("Warning: could not scan %s sessions in %s: %v", source.Name, root, err))
				continue
			}

			for _, file := range files {
				summary, err := source.Parser(file, source.Name)
				if err != nil {
					warn(fmt.Sprintf("Warning: skipping %s session %s: %v", source.Name, file, err))
					continue
				}

				if isEmptySummary(summary) {
					continue
				}

				summaries = append(summaries, summary)
				counts[source.Name]++
				status.Count++
			}
		}

		if status.Count > 0 {
			status.State = "found"
		}
		statuses = append(statuses, status)
	}

	return summaries, counts, statuses
}

func collectGitCommits(now time.Time) ([]SessionSummary, SourceStatus) {
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	output, err := runGitLog(startOfToday)
	status := SourceStatus{Name: "Git Commits", Label: "commits", State: "none"}
	if err != nil {
		status.State = "error"
		status.Error = err.Error()
		return nil, status
	}

	summaries := parseGitLogOutput(output)
	addCommitSnippets(summaries)
	status.Count = len(summaries)
	if status.Count > 0 {
		status.State = "found"
	}
	return summaries, status
}

func parseGitLogOutput(output string) []SessionSummary {
	var summaries []SessionSummary
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) != 4 {
			continue
		}

		summaries = append(summaries, SessionSummary{
			SourceName:    "Git Commits",
			FilePath:      parts[0],
			CommitHash:    parts[0],
			CommitAuthor:  parts[1],
			CommitTime:    parts[2],
			CommitSubject: parts[3],
			ToolsUsed:     map[string]bool{},
		})
	}
	return summaries
}

func addCommitSnippets(summaries []SessionSummary) {
	for index := range summaries {
		if summaries[index].CommitHash == "" {
			continue
		}
		snippet, err := runGitShow(summaries[index].CommitHash)
		if err != nil {
			continue
		}
		summaries[index].CommitSnippet = truncateSnippet(snippet)
	}
}

func truncateSnippet(snippet string) string {
	snippet = strings.TrimSpace(snippet)
	if len(snippet) <= maxCommitSnippetLength {
		return snippet
	}
	return snippet[:maxCommitSnippetLength] + "\n..."
}

func isEmptySummary(summary SessionSummary) bool {
	return len(summary.UserMessages) == 0 && len(summary.AssistantMessages) == 0 && len(summary.ToolsUsed) == 0 && summary.CommitHash == ""
}

func findTodaySessionFiles(root string) ([]string, error) {
	var files []string

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jsonl" && ext != ".json" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if info.ModTime().After(startOfToday) {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

func parseClaudeSessionFile(path string, sourceName string) (SessionSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer file.Close()

	summary := SessionSummary{
		SourceName: sourceName,
		FilePath:   path,
		ToolsUsed:  map[string]bool{},
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Bytes()

		var record ClaudeRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}

		if record.Message == nil {
			continue
		}

		texts := extractTextFromContent(record.Message.Content)
		tools := extractToolNamesFromContent(record.Message.Content)

		for _, tool := range tools {
			summary.ToolsUsed[tool] = true
		}

		switch record.Message.Role {
		case "user":
			summary.UserMessages = append(summary.UserMessages, texts...)
		case "assistant":
			summary.AssistantMessages = append(summary.AssistantMessages, texts...)
		}
	}

	if err := scanner.Err(); err != nil {
		return SessionSummary{}, err
	}

	return summary, nil
}

func parseGenericSessionFile(path string, sourceName string) (SessionSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionSummary{}, err
	}

	summary := SessionSummary{
		SourceName: sourceName,
		FilePath:   path,
		ToolsUsed:  map[string]bool{},
	}

	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		applyGenericRecord(&summary, record)
	}

	if !isEmptySummary(summary) {
		return summary, nil
	}

	var records []map[string]any
	if err := json.Unmarshal(data, &records); err == nil {
		for _, record := range records {
			applyGenericRecord(&summary, record)
		}
		return summary, nil
	}

	var record map[string]any
	if err := json.Unmarshal(data, &record); err == nil {
		applyGenericRecord(&summary, record)
	}

	return summary, nil
}

func applyGenericRecord(summary *SessionSummary, record map[string]any) {
	role, content := genericRoleAndContent(record)
	texts := extractTextFromAny(content)
	if len(texts) == 0 {
		texts = extractTextFromAny(record["text"])
	}

	for _, tool := range extractToolNamesFromAny(content) {
		summary.ToolsUsed[tool] = true
	}
	for _, tool := range extractToolNamesFromAny(record) {
		summary.ToolsUsed[tool] = true
	}

	role = strings.ToLower(role)
	if role == "" {
		typeName, _ := record["type"].(string)
		typeName = strings.ToLower(typeName)
		if strings.Contains(typeName, "user") {
			role = "user"
		} else if strings.Contains(typeName, "assistant") || strings.Contains(typeName, "agent") {
			role = "assistant"
		}
	}

	switch role {
	case "user", "human":
		summary.UserMessages = append(summary.UserMessages, texts...)
	case "assistant", "agent", "ai":
		summary.AssistantMessages = append(summary.AssistantMessages, texts...)
	}
}

func genericRoleAndContent(record map[string]any) (string, any) {
	if message, ok := record["message"].(map[string]any); ok {
		role, _ := message["role"].(string)
		content := message["content"]
		if content == nil {
			content = message["text"]
		}
		return role, content
	}

	role, _ := record["role"].(string)
	content := record["content"]
	if content == nil {
		content = record["text"]
	}
	return role, content
}

func extractTextFromContent(raw json.RawMessage) []string {
	var results []string

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		clean := strings.TrimSpace(asString)
		if clean != "" {
			results = append(results, clean)
		}
		return results
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, block := range blocks {
			blockType, _ := block["type"].(string)

			if blockType == "text" {
				text, _ := block["text"].(string)
				text = strings.TrimSpace(text)

				if text != "" {
					results = append(results, text)
				}
			}
		}
	}

	return results
}

func extractTextFromAny(value any) []string {
	var results []string

	switch typed := value.(type) {
	case nil:
		return results
	case string:
		clean := strings.TrimSpace(typed)
		if clean != "" {
			results = append(results, clean)
		}
	case []any:
		for _, item := range typed {
			results = append(results, extractTextFromAny(item)...)
		}
	case map[string]any:
		if blockType, _ := typed["type"].(string); blockType == "tool_use" || blockType == "function_call" || blockType == "tool" {
			return results
		}
		if text, ok := typed["text"]; ok {
			results = append(results, extractTextFromAny(text)...)
		}
		if content, ok := typed["content"]; ok {
			results = append(results, extractTextFromAny(content)...)
		}
	case json.RawMessage:
		results = append(results, extractTextFromContent(typed)...)
	}

	return results
}

func extractToolNamesFromContent(raw json.RawMessage) []string {
	var tools []string

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return tools
	}

	for _, block := range blocks {
		tools = append(tools, extractToolNamesFromAny(block)...)
	}

	return tools
}

func extractToolNamesFromAny(value any) []string {
	var tools []string

	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			tools = append(tools, extractToolNamesFromAny(item)...)
		}
	case map[string]any:
		blockType, _ := typed["type"].(string)
		if blockType == "tool_use" || blockType == "function_call" || blockType == "tool" {
			if name, _ := typed["name"].(string); name != "" {
				tools = append(tools, name)
			}
		}
		if tool, _ := typed["tool"].(string); tool != "" {
			tools = append(tools, tool)
		}
		if functionCall, ok := typed["function_call"].(map[string]any); ok {
			if name, _ := functionCall["name"].(string); name != "" {
				tools = append(tools, name)
			}
		}
		for _, nestedKey := range []string{"content", "message", "tool_calls"} {
			if nested, ok := typed[nestedKey]; ok {
				tools = append(tools, extractToolNamesFromAny(nested)...)
			}
		}
	case json.RawMessage:
		tools = append(tools, extractToolNamesFromContent(typed)...)
	}

	return tools
}

func writeMarkdownLog(outputFile string, date string, summaries []SessionSummary, statuses []SourceStatus) error {
	if dir := filepath.Dir(outputFile); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	existing, exists, err := readExistingLog(outputFile)
	if err != nil {
		return err
	}

	summaries = filterDuplicateSummaries(existing, summaries)
	if len(summaries) == 0 {
		return nil
	}

	var builder strings.Builder
	if exists {
		builder.WriteString("\n\n")
		builder.WriteString(fmt.Sprintf("## Run - %s\n\n", time.Now().Format("15:04:05")))
	} else {
		builder.WriteString(fmt.Sprintf("# Work Log - %s\n\n", date))
	}

	writeMarkdownStatus(&builder, statuses)
	writeMarkdownSummaries(&builder, summaries)

	flag := os.O_CREATE | os.O_WRONLY
	if exists {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	file, err := os.OpenFile(outputFile, flag, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(builder.String())
	return err
}

func readExistingLog(outputFile string) (string, bool, error) {
	data, err := os.ReadFile(outputFile)
	if err == nil {
		return string(data), true, nil
	}
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return "", false, err
}

func filterDuplicateSummaries(existing string, summaries []SessionSummary) []SessionSummary {
	if strings.TrimSpace(existing) == "" {
		return summaries
	}

	var filtered []SessionSummary
	for _, summary := range summaries {
		if isDuplicateSummary(existing, summary) {
			continue
		}
		filtered = append(filtered, summary)
	}
	return filtered
}

func isDuplicateSummary(existing string, summary SessionSummary) bool {
	if summary.CommitHash != "" {
		return strings.Contains(existing, fmt.Sprintf("`%s`", summary.CommitHash))
	}

	if summary.FilePath != "" {
		return strings.Contains(existing, fmt.Sprintf("### Session: `%s`", filepath.Base(summary.FilePath)))
	}

	return false
}

func writeMarkdownSummaries(builder *strings.Builder, summaries []SessionSummary) {
	bySource := map[string][]SessionSummary{}
	var sourceOrder []string
	for _, summary := range summaries {
		source := summary.SourceName
		if source == "" {
			source = "Claude Code"
		}
		if _, ok := bySource[source]; !ok {
			sourceOrder = append(sourceOrder, source)
		}
		bySource[source] = append(bySource[source], summary)
	}

	for _, source := range sourceOrder {
		if source == "Git Commits" {
			writeGitCommitsSection(builder, bySource[source])
			continue
		}
		writeAgentSessionsSection(builder, source, bySource[source])
	}
}

func writeMarkdownStatus(builder *strings.Builder, statuses []SourceStatus) {
	if len(statuses) == 0 {
		return
	}

	builder.WriteString("## Status\n\n")
	builder.WriteString("| Source | Status |\n")
	builder.WriteString("| --- | --- |\n")
	for _, status := range statuses {
		builder.WriteString(fmt.Sprintf("| %s | %s |\n", status.Name, statusText(status)))
	}
	builder.WriteString("\n")
}

func writeAgentSessionsSection(builder *strings.Builder, source string, summaries []SessionSummary) {
	builder.WriteString(fmt.Sprintf("## %s Sessions\n\n", source))

	for _, summary := range summaries {
		builder.WriteString(fmt.Sprintf("### Session: `%s`\n\n", filepath.Base(summary.FilePath)))

		if len(summary.UserMessages) > 0 {
			builder.WriteString("#### User Requests\n\n")

			for _, msg := range limitStrings(summary.UserMessages, 5) {
				builder.WriteString(fmt.Sprintf("- %s\n", oneLine(msg, 180)))
			}

			builder.WriteString("\n")
		}

		if len(summary.AssistantMessages) > 0 {
			builder.WriteString("#### Assistant Work\n\n")

			for _, msg := range limitStrings(summary.AssistantMessages, 5) {
				builder.WriteString(fmt.Sprintf("- %s\n", oneLine(msg, 180)))
			}

			builder.WriteString("\n")
		}

		if len(summary.ToolsUsed) > 0 {
			builder.WriteString("#### Tools Used\n\n")

			for tool := range summary.ToolsUsed {
				builder.WriteString(fmt.Sprintf("- `%s`\n", tool))
			}

			builder.WriteString("\n")
		}
	}
}

func writeGitCommitsSection(builder *strings.Builder, summaries []SessionSummary) {
	builder.WriteString("## Git Commits\n\n")
	for _, summary := range summaries {
		builder.WriteString(fmt.Sprintf("### `%s` %s\n\n", summary.CommitHash, summary.CommitSubject))
		builder.WriteString(fmt.Sprintf("- Author: %s\n", summary.CommitAuthor))
		builder.WriteString(fmt.Sprintf("- Time: %s\n\n", summary.CommitTime))
		if strings.TrimSpace(summary.CommitSnippet) != "" {
			builder.WriteString("```diff\n")
			builder.WriteString(summary.CommitSnippet)
			builder.WriteString("\n```\n\n")
		}
	}
}

func formatStatusBar(statuses []SourceStatus) string {
	var parts []string
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%s: %s", status.Name, statusText(status)))
	}
	return "Status: " + strings.Join(parts, " | ")
}

func statusText(status SourceStatus) string {
	switch status.State {
	case "found":
		label := status.Label
		if label == "" {
			label = "items"
		}
		return fmt.Sprintf("%d %s", status.Count, label)
	case "missing":
		return "Missing"
	case "error":
		if status.Error != "" {
			return "Error: " + status.Error
		}
		return "Error"
	default:
		return fmt.Sprintf("0 %s", status.Label)
	}
}

func formatCounts(counts map[string]int) string {
	var parts []string
	for _, source := range []string{"Claude Code", "Codex", "Pi", "Git Commits"} {
		if count := counts[source]; count > 0 {
			label := "session(s)"
			if source == "Git Commits" {
				label = "commit(s)"
			}
			parts = append(parts, fmt.Sprintf("%d %s %s", count, source, label))
		}
	}
	if len(parts) == 0 {
		return "0 sessions or commits"
	}
	return strings.Join(parts, ", ")
}

func limitStrings(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}

	return items[:limit]
}

func oneLine(input string, maxLength int) string {
	clean := strings.Join(strings.Fields(input), " ")

	if len(clean) <= maxLength {
		return clean
	}

	return clean[:maxLength] + "..."
}
