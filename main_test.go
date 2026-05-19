package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveOutputFileUsesObsidianVaultWhenSet(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT", filepath.Join("tmp", "vault"))

	got := resolveOutputFile("2026-05-18", "")
	want := filepath.Join("tmp", "vault", "work-log-2026-05-18.md")

	if got != want {
		t.Fatalf("resolveOutputFile() = %q, want %q", got, want)
	}
}

func TestResolveOutputFileFallsBackToCurrentDirectory(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT", "")

	got := resolveOutputFile("2026-05-18", "")
	want := "work-log-2026-05-18.md"

	if got != want {
		t.Fatalf("resolveOutputFile() = %q, want %q", got, want)
	}
}

func TestResolveOutputFileVaultArgumentOverridesEnvironment(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT", filepath.Join("tmp", "env-vault"))

	got := resolveOutputFile("2026-05-18", filepath.Join("tmp", "flag-vault"))
	want := filepath.Join("tmp", "flag-vault", "work-log-2026-05-18.md")

	if got != want {
		t.Fatalf("resolveOutputFile() = %q, want %q", got, want)
	}
}

func TestWriteMarkdownLogGroupsSessionsBySourceAndIncludesStatus(t *testing.T) {
	output := filepath.Join(t.TempDir(), "work-log.md")
	summaries := []SessionSummary{
		{SourceName: "Claude Code", FilePath: "claude.jsonl", UserMessages: []string{"claude request"}, ToolsUsed: map[string]bool{}},
		{SourceName: "Codex", FilePath: "codex.jsonl", UserMessages: []string{"codex request"}, ToolsUsed: map[string]bool{}},
	}
	statuses := []SourceStatus{
		{Name: "Claude Code", Label: "sessions", State: "found", Count: 1},
		{Name: "Codex", Label: "sessions", State: "found", Count: 1},
		{Name: "Pi", Label: "sessions", State: "missing"},
	}

	if err := writeMarkdownLog(output, "2026-05-18", summaries, statuses); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	for _, want := range []string{"## Status", "| Pi | Missing |", "## Claude Code Sessions", "## Codex Sessions", "claude request", "codex request"} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
}

func TestWriteMarkdownLogAppendsAndDeduplicatesExistingEntries(t *testing.T) {
	output := filepath.Join(t.TempDir(), "work-log.md")
	existing := strings.Join([]string{
		"# Work Log - 2026-05-18",
		"",
		"## Claude Code Sessions",
		"",
		"### Session: `claude.jsonl`",
		"",
		"## Git Commits",
		"",
		"- `abc123` existing commit — Eli, 2026-05-18T10:00:00-05:00",
		"",
	}, "\n")
	if err := os.WriteFile(output, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	summaries := []SessionSummary{
		{SourceName: "Claude Code", FilePath: "claude.jsonl", UserMessages: []string{"duplicate session"}, ToolsUsed: map[string]bool{}},
		{SourceName: "Codex", FilePath: "codex.jsonl", UserMessages: []string{"new session"}, ToolsUsed: map[string]bool{}},
		{SourceName: "Git Commits", FilePath: "abc123", CommitHash: "abc123", CommitSubject: "duplicate commit", CommitAuthor: "Eli", ToolsUsed: map[string]bool{}},
		{SourceName: "Git Commits", FilePath: "def456", CommitHash: "def456", CommitSubject: "new commit", CommitAuthor: "Eli", ToolsUsed: map[string]bool{}},
	}
	statuses := []SourceStatus{{Name: "Codex", Label: "sessions", State: "found", Count: 1}}

	if err := writeMarkdownLog(output, "2026-05-18", summaries, statuses); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if strings.Count(text, "### Session: `claude.jsonl`") != 1 {
		t.Fatalf("duplicate session was appended:\n%s", text)
	}
	if strings.Count(text, "`abc123`") != 1 {
		t.Fatalf("duplicate commit was appended:\n%s", text)
	}
	for _, want := range []string{"## Run -", "new session", "`def456` new commit"} {
		if !strings.Contains(text, want) {
			t.Fatalf("appended log missing %q:\n%s", want, text)
		}
	}
}

func TestExistingCandidateRootsReturnsOnlyExistingDirectories(t *testing.T) {
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	got := existingCandidateRoots([]string{existing, missing})

	if len(got) != 1 || got[0] != existing {
		t.Fatalf("existingCandidateRoots() = %#v, want [%q]", got, existing)
	}
}

func TestDefaultSourcesIncludeClaudeCodexPiAndGit(t *testing.T) {
	sources := defaultSources()
	var names []string
	for _, source := range sources {
		names = append(names, source.Name)
	}

	for _, want := range []string{"Claude Code", "Codex", "Pi", "Git Commits"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("defaultSources() names = %#v, missing %q", names, want)
		}
	}
}

func TestParseGenericSessionFileExtractsCommonMessageShapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := strings.Join([]string{
		`{"role":"user","content":"build a feature"}`,
		`{"message":{"role":"assistant","content":[{"type":"text","text":"implemented it"},{"type":"tool_use","name":"read"}]}}`,
	}, "\n")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := parseGenericSessionFile(path, "Codex")
	if err != nil {
		t.Fatal(err)
	}

	if summary.SourceName != "Codex" {
		t.Fatalf("SourceName = %q, want Codex", summary.SourceName)
	}
	if len(summary.UserMessages) != 1 || summary.UserMessages[0] != "build a feature" {
		t.Fatalf("UserMessages = %#v", summary.UserMessages)
	}
	if len(summary.AssistantMessages) != 1 || summary.AssistantMessages[0] != "implemented it" {
		t.Fatalf("AssistantMessages = %#v", summary.AssistantMessages)
	}
	if !summary.ToolsUsed["read"] {
		t.Fatalf("ToolsUsed = %#v, want read", summary.ToolsUsed)
	}
}

func TestCollectTodaySummariesWarnsAndContinuesForMissingSources(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(path, []byte(`{"role":"user","content":"hello"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	var warnings []string
	sources := []SessionSource{
		{Name: "Codex", Label: "sessions", CandidateRoots: []string{root}, Parser: parseGenericSessionFile},
		{Name: "Pi", Label: "sessions", CandidateRoots: []string{missing}, Parser: parseGenericSessionFile},
	}

	summaries, counts, statuses := collectTodaySummaries(sources, func(msg string) { warnings = append(warnings, msg) })

	if len(summaries) != 1 {
		t.Fatalf("summaries len = %d, want 1", len(summaries))
	}
	if counts["Codex"] != 1 {
		t.Fatalf("counts = %#v, want Codex count 1", counts)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "Pi") {
		t.Fatalf("warnings = %#v, want Pi warning", warnings)
	}
	if len(statuses) != 2 || statuses[1].State != "missing" {
		t.Fatalf("statuses = %#v, want missing Pi status", statuses)
	}
}

func TestParseGitLogOutputCreatesCommitSummaries(t *testing.T) {
	input := "abc123\x1fEli\x1f2026-05-18T10:30:00-05:00\x1fadd codex reader\n"

	summaries := parseGitLogOutput(input)

	if len(summaries) != 1 {
		t.Fatalf("summaries len = %d, want 1", len(summaries))
	}
	got := summaries[0]
	if got.SourceName != "Git Commits" || got.CommitHash != "abc123" || got.CommitSubject != "add codex reader" || got.CommitAuthor != "Eli" {
		t.Fatalf("git summary = %#v", got)
	}
}

func TestCollectGitCommitsUsesRunnerAndReportsStatus(t *testing.T) {
	oldRunner := runGitLog
	oldShowRunner := runGitShow
	runGitLog = func(since time.Time) (string, error) {
		return "def456\x1fEli\x1f2026-05-18T11:00:00-05:00\x1fwrite work log\n", nil
	}
	runGitShow = func(hash string) (string, error) {
		return "diff --git a/main.go b/main.go\n+added line\n", nil
	}
	defer func() {
		runGitLog = oldRunner
		runGitShow = oldShowRunner
	}()

	summaries, status := collectGitCommits(time.Now())

	if len(summaries) != 1 {
		t.Fatalf("summaries len = %d, want 1", len(summaries))
	}
	if summaries[0].CommitSnippet == "" || !strings.Contains(summaries[0].CommitSnippet, "+added line") {
		t.Fatalf("CommitSnippet = %q, want diff snippet", summaries[0].CommitSnippet)
	}
	if status.Name != "Git Commits" || status.State != "found" || status.Count != 1 {
		t.Fatalf("status = %#v", status)
	}
}

func TestTruncateSnippetLimitsLongDiffs(t *testing.T) {
	long := strings.Repeat("a", maxCommitSnippetLength+20)

	got := truncateSnippet(long)

	if len(got) > maxCommitSnippetLength+len("\n...") {
		t.Fatalf("truncated snippet too long: %d", len(got))
	}
	if !strings.HasSuffix(got, "\n...") {
		t.Fatalf("truncated snippet = %q, want ellipsis suffix", got)
	}
}
