# OpsAgent-AI

A single-binary CLI that diagnoses the **current machine** using natural language. Ask questions like *"why is my server slow"* or *"check disk usage"* and get an AI-powered diagnosis backed by real system metrics (CPU, memory, disk, processes).

## Architecture

OpsAgent-AI is one Go application using [Google ADK for Go](https://github.com/google/adk-go):

- **CLI (Cobra)** – Parses your query and invokes the agent in-process.
- **ADK Agent (Gemini, OpenAI, or Anthropic)** – Decides which diagnostic tools to run and turns their output into a human-readable diagnosis.
- **Diagnostic tools** – Four in-process tools that gather CPU, memory, disk, and process data using OS-native commands (Linux, macOS, and Windows).

```
┌─────────────────────────────────────────────────────────┐
│  opsagent (single binary)                             │
│  ┌─────────┐    ┌─────────────┐    ┌─────────────────┐  │
│  │  CLI    │───►│  ADK Agent  │───►│ check_cpu       │  │
│  │ (Cobra) │    │  (LLM)      │    │ check_memory    │  │
│  │         │◄───│             │◄───│ check_disk      │  │
│  └─────────┘    └─────────────┘    │ check_processes  │  │
│                                    └─────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

Diagnostics run on the machine where you execute the binary. No separate server or Python process.

## Requirements

- **Go** 1.21+ (to build)
- **API key** for your chosen provider
- **Linux, macOS, or Windows**
  - Linux: uses `top`, `free`, `df`, `ps`
  - macOS: uses `top`, `vm_stat`, `df`, `ps`
  - Windows: uses PowerShell (`Get-CimInstance`, `Get-Process`)

## Installation

### Quick install

From GitHub:

```bash
curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent/main/install.sh | bash
```

Or clone and run from project root:

```bash
git clone https://github.com/josephpaul/opsagent.git && cd opsagent && ./install.sh
```

The script builds the binary and copies it to `/usr/local/bin`. Then run `opsagent config set` to save your API key.

### Manual build

```bash
cd opsagent
go build -o opsagent .
sudo cp opsagent /usr/local/bin/   # Linux/macOS (optional)
```

On Windows (PowerShell):

```powershell
cd opsagent
go build -o opsagent.exe .
```

## Quick Start

```bash
# 1. One-time setup — choose your provider and paste your API key:
opsagent config set

# 2. Run from anywhere:
opsagent "why is my server slow"
opsagent "check cpu"
opsagent "check disk usage"
```

That's it. No `export`, no `.env`, and no manual config-file editing.

## Usage

```bash
opsagent "check memory"
opsagent "check top processes"
opsagent --provider openai --model gpt-4o "check disk usage"
```

Flags:

- **--provider** – `gemini`, `openai`, or `anthropic`
- **--model** – Model name; defaults: `gemini-2.5-flash` (Gemini), `gpt-4o` (OpenAI), `claude-sonnet-4-20250514` (Anthropic)

The CLI auto-detects the provider from whichever key is configured (no `--provider` needed if only one key exists).

## Configuration

### Interactive setup (recommended)

```bash
opsagent config set
```

Walks you through choosing a provider and entering your API key.

### Other config commands

```bash
opsagent config set-provider openai
opsagent config set-model gpt-4o
opsagent config set-base-url https://api.openai.com/v1
opsagent config show          # Show current config (keys masked)
opsagent config path          # Print config file location
opsagent config set-key KEY VALUE  # Set a specific key directly
opsagent config unset KEY     # Remove a saved key
```

### Config file location

| OS | Path |
|---|---|
| macOS | `~/.config/opsagent/config.yaml` |
| Linux | `~/.config/opsagent/config.yaml` |
| Windows | `%APPDATA%\opsagent\config.yaml` |

### Supported keys

| Variable | Provider |
|---|---|
| `GOOGLE_API_KEY` | Gemini |
| `OPENAI_API_KEY` | OpenAI |
| `ANTHROPIC_API_KEY` | Anthropic |
| `OPENAI_BASE_URL` | OpenAI-compatible APIs |
| `OPSAGENT_PROVIDER` | Default provider |
| `OPSAGENT_MODEL` | Default model |

All of these can be set from the CLI, so you never need a `.env` file.

## Background Monitoring

OpsAgent can run as a background service that continuously monitors CPU and RAM usage. When thresholds are breached, it automatically invokes the AI agent to diagnose the cause.

### Run in foreground

```bash
opsagent monitor run --interval 30s --cpu-threshold 90 --ram-threshold 85
opsagent monitor run --interval 5m --log /var/log/opsagent/monitor.log
```

Press Ctrl+C to stop.

### Install as a background service

```bash
opsagent monitor install --interval 60s --cpu-threshold 90 --ram-threshold 85
```

This creates a **systemd** user service on Linux or a **launchd** agent on macOS that starts automatically on login.

### Manage the service

```bash
opsagent monitor status      # Check if running
opsagent monitor stop        # Stop the service
opsagent monitor start       # Start the service
opsagent monitor uninstall   # Remove the service entirely
```

### Monitor flags

| Flag | Default | Description |
|---|---|---|
| `--interval` | `60s` | How often to poll (e.g. `30s`, `5m`, `1h`) |
| `--cpu-threshold` | `90` | CPU % that triggers an AI diagnosis |
| `--ram-threshold` | `85` | RAM % that triggers an AI diagnosis |
| `--log` | *(stdout only)* | Also write logs to this file |

The installer (`install.sh`) also offers to set up monitoring during installation.

## License

Open source. Use and modify as needed.

## Contributing

Contributions are welcome. Please open an issue or PR on the repository.
