# AISH

**One shell for every AI.**

AISH is a single-binary terminal assistant for Linux, Windows, macOS, and WSL. It connects to Ollama, llama.cpp, OpenAI, Claude, and Gemini; keeps local chat sessions; searches local documents; and can propose and run terminal commands only after approval.

> Early development release. Review every proposed command. AISH is not an operating-system sandbox.

## Quick start: Ollama on Windows, AISH in WSL

```bash
chmod +x aish-linux-amd64
./aish-linux-amd64 setup
./aish-linux-amd64 doctor
./aish-linux-amd64
```

During `setup`, AISH attempts to detect the Windows host and Ollama automatically. For a manual setup:

During setup, choose the provider by number:

```text
AISH setup
Choose an AI provider:
  1) ollama   *
  2) llamacpp
  3) openai
  4) claude
  5) gemini
Provider number [ollama]: 1
```


```bash
./aish-linux-amd64 provider use ollama
./aish-linux-amd64 config set model qwen3.5:0.8b
./aish-linux-amd64 config set base-url http://172.22.160.1:11434
./aish-linux-amd64 doctor
```

## Main commands

```text
aish                         Interactive chat
aish ask "QUESTION"          Ask one question
aish do "TASK"               Generate, approve, execute, and explain a command
aish setup                   Guided setup and WSL Ollama detection
aish doctor                  Check connectivity and configured model
aish session list            List saved sessions
aish session open NAME       Continue a saved session
aish docs add PATH           Index local documents
aish docs list               List indexed documents
aish docs remove PATH        Remove a file or folder from the index
aish docs clear              Remove all indexed documents
aish docs search QUERY       Semantic search
aish docs ask QUESTION       Answer using local files
aish provider use NAME       Switch AI provider
aish history                 Show recent local history
aish run "COMMAND"           Run an exact command after approval
aish version                 Show version
```


## Agent mode (v0.8)

AISH can create a multi-step plan using the current folder and Git repository as context:

```bash
aish agent "inspect this Go project, run tests, and summarize failures"
```

Each step shows its command and requires one of: approve, skip, pause, or cancel. Tasks are saved locally and can be resumed:

```bash
aish agent list
aish agent show TASK_ID
aish agent resume TASK_ID
aish agent delete TASK_ID
```

Failed commands are sent back to the active model for a corrected proposal. AISH asks before retrying the correction.

Project and Git context can be inspected directly:

```bash
aish project context
```

## Personal knowledge (v0.9)

Create separate knowledge collections for personal, work, study, or individual projects:

```bash
aish knowledge create personal
aish knowledge create work
aish knowledge list
aish knowledge use personal
aish knowledge add ~/Documents
aish knowledge search "insurance renewal"
aish knowledge ask "When does my insurance renew?"
```

Answers are instructed to cite indexed source paths. Watch a folder and automatically re-index changed files until Ctrl+C:

```bash
aish knowledge watch ~/Documents/notes
```

Manage collection content:

```bash
aish knowledge remove ~/Documents/old-note.md
aish knowledge clear personal
aish knowledge delete work
```

### Optional encrypted local storage

Set an encryption passphrase before using AISH:

```bash
export AISH_ENCRYPTION_KEY='a long private passphrase'
aish privacy
```

When set, newly written history, sessions, agent state, collection metadata, and vector indexes are encrypted locally with AES-GCM. Keep the passphrase safe: encrypted data cannot be opened without it. Existing unencrypted data remains readable and is encrypted the next time it is saved.

## Natural-language commands

```bash
aish do "show the ten largest files in this directory"
```

AISH asks the model for one structured command, validates it, displays the command and reason, asks for approval, executes it with a timeout, and asks the model to explain the output.

## Persistent sessions

```bash
aish chat --session project-x
# inside chat: /save project-x

aish session list
aish session open project-x
aish session delete project-x
```

Sessions and history are stored locally in the OS cache directory.

## Local documents

Install an Ollama embedding model:

```bash
ollama pull nomic-embed-text
```

Then index and query files:

```bash
aish docs add ./documents
aish docs list
aish docs remove ./documents/old-notes.md
aish docs clear
aish docs search "deployment"
aish docs ask "How is the project deployed?"
```

Supported text/code formats include TXT, Markdown, JSON, CSV, YAML, TOML, XML, HTML, CSS, Go, Python, JavaScript, TypeScript, Rust, Java, C, and C++. DOCX is extracted natively. PDF requires `pdftotext` from Poppler (`sudo apt install poppler-utils` on Debian/Ubuntu/WSL).

Re-indexing a file replaces its old chunks rather than creating duplicates. Embeddings and document text remain local.

## Providers

```bash
aish provider list
aish provider use ollama
aish provider use llamacpp
aish provider use openai
aish provider use claude
aish provider use gemini
```

Cloud providers read keys from environment variables:

```bash
export OPENAI_API_KEY='...'
export ANTHROPIC_API_KEY='...'
export GEMINI_API_KEY='...'
```

## Installation

AISH has no runtime dependency and ships as one executable.

Linux/macOS after publishing GitHub releases:

```bash
curl -fsSL https://raw.githubusercontent.com/khashino/AISH/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/khashino/AISH/main/scripts/install.ps1 | iex
```

## Build

Requires Go 1.22 or newer:

```bash
go test ./...
go build -o aish ./cmd/aish
```

## Release

Push a version tag. GitHub Actions builds Linux AMD64/ARM64, Windows AMD64, and macOS AMD64/ARM64 binaries and attaches them to a GitHub release.

```bash
git tag v0.9.0
git push origin v0.9.0
```

## Security

- Commands require approval by default.
- Known destructive patterns are blocked.
- Commands time out after 60 seconds.
- Cloud API keys are read from environment variables, not stored in configuration.
- Command validation is defense-in-depth, not a complete sandbox.

## Roadmap

- Stronger command sandboxing and policy profiles
- Page-aware PDF citations and richer DOCX metadata
- Incremental hashing for very large knowledge bases
- Signed release checksums and update verification
- Shell completion and package-manager manifests

## v0.9.1 agent reliability

Agent mode now includes deterministic plans for common Go project inspection tasks, rejects invalid project initialization commands when `go.mod` already exists, limits failed-command correction retries, saves interrupted tasks as paused, and accepts either the displayed task number or full task ID for `show`, `resume`, and `delete`.

```bash
aish agent list
aish agent show 1
aish agent resume 1
aish agent delete 1
```

## Token usage and cost tracking (v1.0)

AISH records token usage metadata for chat, one-shot questions, command planning, document answers, and agent tasks. Prompts and answers are not copied into the usage log.

After a request, the default compact display is:

```text
[~1,558 tokens · 2.80s · ollama/qwen3.5:0.8b]
```

A `~` means the count is estimated. Providers that return usage metadata are recorded as exact.

```bash
aish usage
aish usage today
aish usage session my-project
aish usage task 1
aish usage export --format json
aish usage export --format csv --output usage.csv
aish usage reset
```

Inside interactive chat:

```text
/usage
```

Control the per-request display:

```bash
aish config set show-usage summary
aish config set show-usage always
aish config set show-usage off
```

Optional cost estimates use user-configured prices per one million tokens for the active provider/model:

```bash
aish pricing show
aish pricing set input 0.15
aish pricing set output 0.60
```

Prices are intentionally not hard-coded because provider pricing changes. Local providers default to `$0.00` API cost. Usage logs are encrypted when `AISH_ENCRYPTION_KEY` is set, using the same encrypted local-storage system as history and indexes.
