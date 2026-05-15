package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
	FilePath          string
	UserMessages      []string
	AssistantMessages []string
	ToolsUsed         map[string]bool
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "log-work",
		Short: "Ingest today's Claude Code sessions into a local work log",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogWork()
		},
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func runLogWork() error {
	today := time.Now().Format("2006-01-02")

	sessionsRoot, err := getClaudeProjectsDir()
	if err != nil {
		return err
	}

	sessionFiles, err := findTodaySessionFiles(sessionsRoot)
	if err != nil {
		return err
	}

	if len(sessionFiles) == 0 {
		fmt.Println("No Claude Code sessions found for today.")
		return nil
	}

	var summaries []SessionSummary

	for _, file := range sessionFiles {
		summary, err := parseSessionFile(file)
		if err != nil {
			fmt.Printf("Skipping %s: %v\n", file, err)
			continue
		}

		summaries = append(summaries, summary)
	}

	outputFile := fmt.Sprintf("work-log-%s.md", today)

	if err := writeMarkdownLog(outputFile, today, summaries); err != nil {
		return err
	}

	fmt.Printf("Found %d Claude Code session(s) from today.\n\n", len(summaries))
	fmt.Println("Work log written to:")
	fmt.Println(outputFile)

	return nil
}

func getClaudeProjectsDir() (string, error) {
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")

	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		configDir = filepath.Join(homeDir, ".claude")
	}

	projectsDir := filepath.Join(configDir, "projects")

	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return "", fmt.Errorf("Claude projects directory not found: %s", projectsDir)
	}

	return projectsDir, nil
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

		if filepath.Ext(path) != ".jsonl" {
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

func parseSessionFile(path string) (SessionSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer file.Close()

	summary := SessionSummary{
		FilePath:  path,
		ToolsUsed: map[string]bool{},
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

func extractTextFromContent(raw json.RawMessage) []string {
	var results []string

	// Sometimes content is a plain string.
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		clean := strings.TrimSpace(asString)
		if clean != "" {
			results = append(results, clean)
		}
		return results
	}

	// Sometimes content is an array of blocks.
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

func extractToolNamesFromContent(raw json.RawMessage) []string {
	var tools []string

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return tools
	}

	for _, block := range blocks {
		blockType, _ := block["type"].(string)

		if blockType == "tool_use" {
			name, _ := block["name"].(string)
			if name != "" {
				tools = append(tools, name)
			}
		}
	}

	return tools
}

func writeMarkdownLog(outputFile string, date string, summaries []SessionSummary) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("# Work Log - %s\n\n", date))
	builder.WriteString("## Claude Code Sessions\n\n")

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

	return os.WriteFile(outputFile, []byte(builder.String()), 0644)
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
