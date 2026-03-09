# AI-TERM

An AI-powered terminal emulator built as a TUI (Terminal User Interface) application in Go. It combines a real interactive shell with multi-provider AI modes (Google Gemini, OpenAI, Anthropic Claude), persistent sessions, and a fully themed interface — all running inside your terminal.

## Features

- **Three operating modes** — Shell, AI chat, and Autonomous task execution
- **Multi-provider AI** — Google Gemini, OpenAI, or Anthropic Claude (auto-detected from environment)
- **Persistent named sessions** — Browser-tab-style chat sessions stored in BoltDB
- **Command history** — Up/down navigation with persistent storage across restarts
- **Inline autocomplete** — Ghost-text suggestions from history and a built-in command library
- **Session sidebar** — Create, switch, rename, and delete sessions
- **Scrollable viewport** — Full message history with scroll position indicator
- **6 built-in themes** — Dracula, Tokyo Night, Gruvbox, Catppuccin, Nord, Solarized
- **Persistent working directory** — Restored automatically on next launch

## Modes

### Shell Mode
Run real bash commands directly in the TUI. Autocomplete draws from your command history and a curated static command library. The working directory persists across sessions.

### AI Mode (`Tab` to switch)
Chat with an AI agent (Gemini, GPT-4o, or Claude) that acts as a Linux sysadmin assistant. The agent has access to your shell and can run commands on your behalf to answer questions or solve problems. Conversation history is maintained within each session.

### Auto Mode (`Ctrl+X` to enable)
Describe a high-level task in natural language (e.g., *"find all .log files larger than 1MB and compress them"*). The autonomous agent decomposes the task into shell commands, executes them step by step, streams output live, and reports when done. Capped at 30 tool calls per task for safety.

## Prerequisites

- Go 1.24.4 or later
- An API key for at least one supported provider:
  - **Google Gemini** — https://aistudio.google.com/apikey
  - **OpenAI** — https://platform.openai.com/api-keys
  - **Anthropic** — https://console.anthropic.com/
- `bash` available in your `PATH`

## Installation

```bash
# Clone the repository
git clone <repo-url>
cd tui-start

# Install dependencies
go mod download
```

## Configuration

Set an API key for your preferred provider. The app auto-detects which provider to use based on which key is present (Google takes priority if multiple keys are set):

```bash
# Google Gemini (default model: gemini-2.5-flash)
export GOOGLE_API_KEY=your_google_key

# OpenAI (default model: gpt-4o)
export OPENAI_API_KEY=your_openai_key

# Anthropic Claude (default model: claude-3-5-sonnet-20241022)
export ANTHROPIC_API_KEY=your_anthropic_key
```

Optional overrides:

```bash
# Force a specific provider (values: google, gemini, openai, anthropic, claude)
export PROVIDER=openai

# Override the model used by the selected provider
export MODEL=gpt-4o-mini
```

You can store these in a `.env` file and source it before running:

```bash
source .env
```

> The app will not start unless at least one API key is set (or `PROVIDER` is explicitly configured with its key).

## Running

```bash
# Run directly
go run .

# Or build and run a binary
go build -o ai-term .
./ai-term
```

On first run, a BoltDB database is created at `~/.config/ai-shell/session.db` to store sessions, messages, command history, theme preference, and working directory.

## Key Bindings

### Global

| Key | Action |
|-----|--------|
| `Tab` | Toggle Shell / AI mode (or accept suggestion) |
| `Ctrl+X` | Toggle Auto-execute mode |
| `Ctrl+T` | Cycle to next colour theme |
| `Ctrl+L` | Clear current session messages |
| `Ctrl+C` | Save state and quit |
| `PgUp` / `PgDn` | Scroll message viewport |

### Sessions

| Key | Action |
|-----|--------|
| `Ctrl+B` | Open / focus sessions sidebar |
| `Ctrl+N` | Create a new session |
| `Enter` (in sidebar) | Switch to highlighted session |
| `Delete` / `Ctrl+D` (in sidebar) | Delete highlighted session |
| `Esc` / `Ctrl+B` (in sidebar) | Close sidebar |

### Input Editing

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate command history (shell mode) |
| `Alt+↑` / `Alt+↓` / `Ctrl+P` | Navigate suggestion list |
| `→` | Accept inline ghost-text suggestion |
| `Esc` | Dismiss suggestions |
| `Ctrl+A` / `Ctrl+E` | Move to start / end of line |
| `Ctrl+K` | Delete to end of line |
| `Ctrl+U` | Delete to start of line |
| `Ctrl+W` | Delete word before cursor |

## Project Structure

```
tui-start/
├── main.go              # Entry point; BubbleTea model, all UI logic
├── main_test.go         # White-box tests for main package helpers
├── export_test.go       # Exports main package internals for testing
├── agents/
│   ├── provider.go      # Provider abstraction: ProviderConfig, LLMClient interface, DetectProvider
│   ├── shell_agent.go   # Interactive AI agent with shell tool access
│   ├── auto_agent.go    # Autonomous multi-step task executor
│   ├── gemini_client.go # Google Gemini backend
│   ├── openai_client.go # OpenAI backend
│   └── anthropic_client.go # Anthropic Claude backend
├── models/
│   └── message.go       # Shared Message type and Role enum
├── storage/
│   └── store.go         # BoltDB persistence (sessions, history, config)
├── test/                # External integration tests (package tuistart_test)
│   ├── agents_test.go   # agents package: provider, shell agent, auto agent
│   ├── models_test.go   # models package: Message and Role
│   ├── storage_test.go  # storage package: CRUD for sessions, messages, history
│   ├── themes_test.go   # themes package: palette and style fields
│   ├── tools_test.go    # tools package: ExecCommand behaviour
│   └── tui_test.go      # tui package: Spinner and Suggester
├── themes/
│   └── themes.go        # 6 built-in colour themes
├── tools/
│   └── shell.go         # Shell command executor (bash wrapper)
└── tui/
    ├── spinner.go        # Animated loading spinner component
    └── suggestion.go     # Command autocomplete / suggestion engine
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run only the integration tests in test/
go test ./test/...

# Run only the main package tests
go test .

# Verbose output
go test ./... -v
```

The test suite currently contains **134 tests** across all packages with 0 failures.

## Dependencies

| Package | Purpose |
|---------|---------|
| `charm.land/bubbletea/v2` | TUI framework (Elm architecture for terminals) |
| `github.com/boltdb/bolt` | Embedded key-value database for persistence |
| `github.com/charmbracelet/lipgloss` | Terminal styling and layout |
| `google.golang.org/genai` | Google Generative AI Go SDK (Gemini API client) |
| `github.com/openai/openai-go` | OpenAI Go SDK |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic Claude Go SDK |

## Data Storage

All persistent data is stored in `~/.config/ai-shell/session.db` (BoltDB). This includes:

- Named sessions and their full message history
- Shell command history
- Active session and working directory
- Theme preference

The database is created automatically on first run. If it cannot be opened, the app falls back to an ephemeral in-memory session.
