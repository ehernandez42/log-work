# log-work

A simple CLI tool that will take your work, via commits and coding agent sessions, and parse it out to a text file for you to store as part of 
your deliverables.

## Usage

By default, `log-work` auto-detects today's Claude Code, Codex, and Pi session files from known local locations. It also reads today's commits from the current git repository, so a work log can still be created when no agent sessions ran.

The generated markdown includes a status table for each source, and the CLI prints the same status summary while running. If today's work-log file already exists, new non-duplicate sessions and commits are appended under a new run section instead of overwriting the file.

Git commits include truncated diff snippets so the log captures what changed without writing full patches into your vault.

Use `--vault` to choose the Obsidian vault for a single run:

```powershell
log-work --vault "C:\\Users\\you\\Documents\\ObsidianVault"
```

The `--vault` argument overrides `OBSIDIAN_VAULT`.

If `OBSIDIAN_VAULT` is set, the log is written there:

```powershell
$env:OBSIDIAN_VAULT = "C:\\Users\\you\\Documents\\ObsidianVault"
go run .
```

Without `OBSIDIAN_VAULT`, the log is written to the current directory.
