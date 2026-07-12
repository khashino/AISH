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



## Security

- Commands require approval by default.
- Known destructive patterns are blocked.
- Commands time out after 60 seconds.
- Cloud API keys are read from environment variables, not stored in configuration.
- Command validation is defense-in-depth, not a complete sandbox.

