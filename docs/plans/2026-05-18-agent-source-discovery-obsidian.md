# Agent Source Discovery and Obsidian Output Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Extend `log-work` to auto-detect Claude, Codex, and Pi session files from today and write the generated markdown log into the Obsidian vault configured by `OBSIDIAN_VAULT` when present.

**Architecture:** Refactor the single Claude-only flow into source definitions that each provide candidate session roots, file discovery, and parsing. Keep Claude parsing specific, add tolerant generic parsing for Codex and Pi, and group output by source. Resolve the output path through `OBSIDIAN_VAULT` with current-directory fallback.

**Tech Stack:** Go 1.26.1, standard library JSON/filepath/os/time packages, Cobra CLI, Go unit tests.

---

### Task 1: Add tests for Obsidian output path resolution

**Files:**
- Create: `main_test.go`
- Modify: `main.go`

**Step 1: Write the failing test**

Create `main_test.go` with tests for a new helper that resolves where the work log should be written:

```go
package main

import (
	"path/filepath"
	"testing"
)

func TestResolveOutputFileUsesObsidianVaultWhenSet(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT", filepath.Join("tmp", "vault"))

	got := resolveOutputFile("2026-05-18")
	want := filepath.Join("tmp", "vault", "work-log-2026-05-18.md")

	if got != want {
		t.Fatalf("resolveOutputFile() = %q, want %q", got, want)
	}
}

func TestResolveOutputFileFallsBackToCurrentDirectory(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT", "")

	got := resolveOutputFile("2026-05-18")
	want := "work-log-2026-05-18.md"

	if got != want {
		t.Fatalf("resolveOutputFile() = %q, want %q", got, want)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./...`

Expected: FAIL because `resolveOutputFile` is undefined.

**Step 3: Write minimal implementation**

In `main.go`, add:

```go
func resolveOutputFile(date string) string {
	fileName := fmt.Sprintf("work-log-%s.md", date)
	vault := strings.TrimSpace(os.Getenv("OBSIDIAN_VAULT"))
	if vault == "" {
		return fileName
	}
	return filepath.Join(vault, fileName)
}
```

Update `runLogWork()` to use:

```go
outputFile := resolveOutputFile(today)
```

instead of formatting the file name inline.

**Step 4: Run test to verify it passes**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: write logs to configured obsidian vault"
```

---

### Task 2: Introduce source grouping in summaries and markdown output

**Files:**
- Modify: `main_test.go`
- Modify: `main.go`

**Step 1: Write the failing test**

Append this test to `main_test.go`:

```go
func TestWriteMarkdownLogGroupsSessionsBySource(t *testing.T) {
	output := filepath.Join(t.TempDir(), "work-log.md")
	summaries := []SessionSummary{
		{SourceName: "Claude Code", FilePath: "claude.jsonl", UserMessages: []string{"claude request"}, ToolsUsed: map[string]bool{}},
		{SourceName: "Codex", FilePath: "codex.jsonl", UserMessages: []string{"codex request"}, ToolsUsed: map[string]bool{}},
	}

	if err := writeMarkdownLog(output, "2026-05-18", summaries); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	for _, want := range []string{"## Claude Code Sessions", "## Codex Sessions", "claude request", "codex request"} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
}
```

Add imports for `os` and `strings` if not already present.

**Step 2: Run test to verify it fails**

Run: `go test ./...`

Expected: FAIL because `SessionSummary.SourceName` does not exist or output is not grouped by source.

**Step 3: Write minimal implementation**

In `main.go`, add `SourceName string` to `SessionSummary`.

Update `writeMarkdownLog` to group summaries by `SourceName` while preserving first-seen source order:

```go
func writeMarkdownLog(outputFile string, date string, summaries []SessionSummary) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("# Work Log - %s\n\n", date))

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
		builder.WriteString(fmt.Sprintf("## %s Sessions\n\n", source))
		for _, summary := range bySource[source] {
			// keep existing per-session rendering here
		}
	}

	return os.WriteFile(outputFile, []byte(builder.String()), 0644)
}
```

Move the existing per-session rendering block inside the source loop.

**Step 4: Run test to verify it passes**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: group work log by agent source"
```

---

### Task 3: Add source abstraction and conservative candidate discovery

**Files:**
- Modify: `main_test.go`
- Modify: `main.go`

**Step 1: Write the failing test**

Append tests for source root discovery:

```go
func TestExistingCandidateRootsReturnsOnlyExistingDirectories(t *testing.T) {
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	got := existingCandidateRoots([]string{existing, missing})

	if len(got) != 1 || got[0] != existing {
		t.Fatalf("existingCandidateRoots() = %#v, want [%q]", got, existing)
	}
}

func TestDefaultSourcesIncludeClaudeCodexAndPi(t *testing.T) {
	sources := defaultSources()
	var names []string
	for _, source := range sources {
		names = append(names, source.Name)
	}

	for _, want := range []string{"Claude Code", "Codex", "Pi"} {
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./...`

Expected: FAIL because `existingCandidateRoots`, `defaultSources`, and `SessionSource` do not exist.

**Step 3: Write minimal implementation**

In `main.go`, add:

```go
type SessionSource struct {
	Name           string
	CandidateRoots []string
	Parser         func(string, string) (SessionSummary, error)
}

func defaultSources() []SessionSource {
	homeDir, _ := os.UserHomeDir()
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")

	return []SessionSource{
		{Name: "Claude Code", CandidateRoots: []string{filepath.Join(homeDir, ".claude", "projects")}, Parser: parseClaudeSessionFile},
		{Name: "Codex", CandidateRoots: []string{filepath.Join(homeDir, ".codex"), filepath.Join(localAppData, "codex"), filepath.Join(appData, "codex")}, Parser: parseGenericSessionFile},
		{Name: "Pi", CandidateRoots: []string{filepath.Join(homeDir, ".pi"), filepath.Join(localAppData, "pi"), filepath.Join(appData, "pi")}, Parser: parseGenericSessionFile},
	}
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
```

Rename `parseSessionFile` to `parseClaudeSessionFile` and update call sites later.

**Step 4: Run test to verify it passes**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: define agent session sources"
```

---

### Task 4: Add generic tolerant parser for Codex and Pi JSON/JSONL files

**Files:**
- Modify: `main_test.go`
- Modify: `main.go`

**Step 1: Write the failing test**

Append tests:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./...`

Expected: FAIL because `parseGenericSessionFile` does not exist.

**Step 3: Write minimal implementation**

Add a generic parser that scans JSONL first and falls back to a single JSON object/array if needed. Use helper functions to extract `role`, `content`, `message.role`, `message.content`, `text`, and tool names.

Suggested helpers:

```go
func parseGenericSessionFile(path string, sourceName string) (SessionSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer file.Close()

	summary := SessionSummary{FilePath: path, SourceName: sourceName, ToolsUsed: map[string]bool{}}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		applyGenericRecord(&summary, record)
	}
	if err := scanner.Err(); err != nil {
		return SessionSummary{}, err
	}
	return summary, nil
}
```

Implement `applyGenericRecord`, `genericRoleAndContent`, and `extractTextFromAny` narrowly enough to pass the test, then refactor to reuse `extractTextFromContent` and `extractToolNamesFromContent` where possible.

**Step 4: Run test to verify it passes**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: parse codex and pi session formats tolerantly"
```

---

### Task 5: Wire all sources into `runLogWork` with warnings and summary counts

**Files:**
- Modify: `main_test.go`
- Modify: `main.go`

**Step 1: Write the failing test**

Add a lower-level test for source collection to avoid needing to capture stdout from the Cobra command:

```go
func TestCollectTodaySummariesWarnsAndContinuesForMissingSources(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(path, []byte(`{"role":"user","content":"hello"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	var warnings []string
	sources := []SessionSource{
		{Name: "Codex", CandidateRoots: []string{root}, Parser: parseGenericSessionFile},
		{Name: "Pi", CandidateRoots: []string{missing}, Parser: parseGenericSessionFile},
	}

	summaries, counts := collectTodaySummaries(sources, func(msg string) { warnings = append(warnings, msg) })

	if len(summaries) != 1 {
		t.Fatalf("summaries len = %d, want 1", len(summaries))
	}
	if counts["Codex"] != 1 {
		t.Fatalf("counts = %#v, want Codex count 1", counts)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "Pi") {
		t.Fatalf("warnings = %#v, want Pi warning", warnings)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./...`

Expected: FAIL because `collectTodaySummaries` does not exist.

**Step 3: Write minimal implementation**

Add:

```go
func collectTodaySummaries(sources []SessionSource, warn func(string)) ([]SessionSummary, map[string]int) {
	var summaries []SessionSummary
	counts := map[string]int{}

	for _, source := range sources {
		roots := existingCandidateRoots(source.CandidateRoots)
		if len(roots) == 0 {
			warn(fmt.Sprintf("Warning: %s sessions directory not found in known locations.", source.Name))
			continue
		}

		for _, root := range roots {
			files, err := findTodaySessionFiles(root)
			if err != nil {
				warn(fmt.Sprintf("Warning: could not scan %s sessions in %s: %v", source.Name, root, err))
				continue
			}
			for _, file := range files {
				summary, err := source.Parser(file, source.Name)
				if err != nil {
					warn(fmt.Sprintf("Warning: skipping %s session %s: %v", source.Name, file, err))
					continue
				}
				if len(summary.UserMessages) == 0 && len(summary.AssistantMessages) == 0 && len(summary.ToolsUsed) == 0 {
					continue
				}
				summaries = append(summaries, summary)
				counts[source.Name]++
			}
		}
	}

	return summaries, counts
}
```

Update `runLogWork()` to call `collectTodaySummaries(defaultSources(), func(msg string) { fmt.Println(msg) })`, write the log to `resolveOutputFile(today)`, and print per-source counts.

Update `findTodaySessionFiles` to accept both `.jsonl` and `.json` files.

**Step 4: Run test to verify it passes**

Run: `go test ./...`

Expected: PASS.

**Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: collect sessions from all agent sources"
```

---

### Task 6: Final verification and README update

**Files:**
- Modify: `README.md`

**Step 1: Update documentation**

Add a short usage section:

```md
## Usage

By default, `log-work` auto-detects today's Claude Code, Codex, and Pi session files from known local locations and writes a markdown work log.

If `OBSIDIAN_VAULT` is set, the log is written there:

```powershell
$env:OBSIDIAN_VAULT = "C:\\Users\\you\\Documents\\ObsidianVault"
go run .
```

Without `OBSIDIAN_VAULT`, the log is written to the current directory.
```

**Step 2: Run full verification**

Run:

```bash
go test ./...
go run .
```

Expected:
- Tests pass.
- `go run .` completes with warnings for missing sources if applicable.
- A `work-log-YYYY-MM-DD.md` file appears in `OBSIDIAN_VAULT` when set, otherwise in the current directory.

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document agent discovery and obsidian output"
```
