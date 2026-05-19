# log-work

A CLI tool that ingests today's coding agent sessions and git commits into a local work-log markdown file — useful as a deliverable or for Obsidian vaults.

## What it does

`log-work` auto-discovers today's session files from Claude Code, Codex, and Pi from their default local paths. It also reads today's commits from the current git repository (including truncated diff snippets). The generated markdown includes a status table per source and appends new non-duplicate sessions/commits under a new run section when the log already exists for today.

## Installation

**Prerequisites:** Go 1.21+

### Linux / WSL

```sh
./build.sh
```

Builds `dist/log-work-linux-amd64` and `dist/log-work.exe`, then installs the Linux binary to `~/.local/bin/log-work`. Override the install directory:

```sh
INSTALL_DIR=/usr/local/bin ./build.sh
```

If `~/.local/bin` is not in your PATH, add to your shell profile:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

### Windows (PowerShell)

```powershell
.\build.ps1
```

Builds both binaries, installs `dist/log-work.exe` to `%USERPROFILE%\bin`, and adds that directory to your user PATH. If WSL is available it also installs the Linux binary to `~/.local/bin/log-work` inside WSL.

Open a new terminal window after running so PATH changes take effect.

## Usage

Run from inside any git repository:

```sh
log-work
```

By default the log is written to `work-log-YYYY-MM-DD.md` in the current directory.

### Write to an Obsidian vault

Pass `--vault` for a single run:

```sh
log-work --vault "/path/to/ObsidianVault"
```

Or set the environment variable so every run goes there:

```sh
export OBSIDIAN_VAULT="/path/to/ObsidianVault"
log-work
```

On Windows:

```powershell
$env:OBSIDIAN_VAULT = "C:\Users\you\Documents\ObsidianVault"
log-work
```

`--vault` takes precedence over `OBSIDIAN_VAULT`.

## Session sources

| Source | Default path |
| --- | --- |
| Claude Code | `~/.claude/projects/` |
| Codex | `~/.codex/`, `%LOCALAPPDATA%\codex`, `%APPDATA%\codex` |
| Pi | `~/.pi/`, `%LOCALAPPDATA%\pi`, `%APPDATA%\pi` |
| Git Commits | current working directory repository |

Sources that are not found are marked **Missing** in the status table rather than causing an error.

## Output

Each run produces (or appends to) a markdown file structured like:

```
# Work Log - 2026-05-19

## Status
| Source       | Status     |
| ---          | ---        |
| Claude Code  | 3 sessions |
| Git Commits  | 2 commits  |

## Claude Code Sessions
### Session: `abc123.jsonl`
#### User Requests
- ...
#### Assistant Work
- ...
#### Tools Used
- `Edit`

## Git Commits
### `a1b2c3d` fix: handle empty vault path
- Author: Jane Dev
- Time: 2026-05-19T10:30:00-05:00

```diff
...
```
```

Running `log-work` a second time on the same day appends a `## Run - HH:MM:SS` section with only the new sessions and commits.
