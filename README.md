# OpsAgent-AI

A single-binary CLI that diagnoses the **current machine** using natural language. Ask questions like *"why is my server slow"* or *"check disk usage"* and get an AI-powered diagnosis backed by real system metrics (CPU, memory, disk, processes).

## Architecture

OpsAgent-AI is one Go application using [Google ADK for Go](https://github.com/google/adk-go):

- **CLI (Cobra)** – Parses your query and invokes the agent in-process.
- **ADK Agent (Gemini, OpenAI, or Anthropic)** – Decides which diagnostic tools to run and turns their output into a human-readable diagnosis.
- **Diagnostic tools** – Four in-process tools that run `top`, `free`/`vm_stat`, `df`, and `ps` on the current machine and return structured data.

```
┌─────────────────────────────────────────────────────────┐
│  opsagent-ai (single binary)                             │
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
- **API key** for your chosen provider: **GOOGLE_API_KEY** (Gemini), **OPENAI_API_KEY** (OpenAI), or **ANTHROPIC_API_KEY** (Anthropic)
- **Linux or macOS** – Diagnostics use `top`/`free`/`df`/`ps` on Linux and `top`/`vm_stat`/`df`/`ps` on macOS

## Installation

### Quick install

From GitHub:

```bash
curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent-ai/main/install.sh | bash
```

Or clone and run from project root:

```bash
git clone https://github.com/josephpaul/opsagent-ai.git && cd opsagent-ai && ./install.sh
```

The script builds the binary and copies it to `/usr/local/bin` (or prints a `sudo cp` command if needed).

### Manual build

```bash
cd opsagent-ai
go build -o opsagent-ai .
sudo cp opsagent-ai /usr/local/bin/   # optional
```

## Usage

Set the API key for your chosen provider (via environment variable or a `.env` file in the current directory), then run the CLI:

```bash
# Option 1: use a .env file (copy .env.example to .env and add your keys)
# Option 2: export in the shell

# Default: Gemini
export GOOGLE_API_KEY=your_key
opsagent-ai "why is my server slow"
opsagent-ai "check cpu"
opsagent-ai "check disk usage"

# OpenAI
export OPENAI_API_KEY=your_key
opsagent-ai --provider openai --model gpt-4o "check memory"

# Anthropic
export ANTHROPIC_API_KEY=your_key
opsagent-ai --provider anthropic --model claude-sonnet-4-20250514 "check top processes"
```

Flags:

- **--provider** – `gemini` (default), `openai`, or `anthropic`
- **--model** – Model name; defaults: `gemini-2.5-flash` (Gemini), `gpt-4o` (OpenAI), `claude-sonnet-4-20250514` (Anthropic)

Output format:

```
--- Diagnosis ---
<one-line summary>

--- Details ---
<additional explanation>
```

## Configuration

- **GOOGLE_API_KEY** (required) – Set in the environment or in a `.env` file in the current directory if you add a small env loader.
- **--model** – Gemini model name (default: `gemini-2.5-flash`).

## License

Open source. Use and modify as needed.

## Contributing

Contributions are welcome. Please open an issue or PR on the repository.
