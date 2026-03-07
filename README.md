# OpsAgent-AI

A CLI tool that lets you diagnose Linux servers using natural language. Ask questions like *"why is my server slow"* or *"check disk usage"* and get an AI-powered diagnosis backed by real system metrics.

## Project overview

OpsAgent-AI has three components that communicate over HTTP:

1. **CLI (Go)** – Accepts your natural language query and calls the AI agent.
2. **AI Agent (Python)** – Uses Google ADK with your choice of **Gemini**, **Anthropic (Claude)**, or **OpenAI** to interpret your question, call the server agent, and return a human-readable diagnosis.
3. **Server Agent (Go)** – Runs on the target server and exposes endpoints that run Linux commands (CPU, memory, disk, processes) and return JSON.

```
┌─────────────┐     POST /query      ┌─────────────┐     GET /cpu, /memory,     ┌─────────────┐
│  opsagent-ai │ ──────────────────►  │  AI Agent   │ ────────────────────────►  │   Server    │
│   (CLI)     │  {"question": "..."} │  (Python)   │  /disk, /processes         │   Agent     │
│   Go        │ ◄──────────────────  │  FastAPI +  │ ◄────────────────────────  │   (Go)      │
└─────────────┘  diagnosis + details  │  Google ADK │  JSON (metrics)             └─────────────┘
                                     └─────────────┘
        User                              :9000                                        :8080
```

## Architecture

| Component    | Language | Port | Role |
|-------------|----------|------|------|
| CLI         | Go (Cobra) | - | Sends `POST /query` to AI agent, prints response |
| AI Agent    | Python (FastAPI, Google ADK) | 9000 | Receives question, calls server-agent APIs, returns diagnosis |
| Server Agent| Go        | 8080 | Runs `top`, `free`, `df`, `ps` and returns JSON |

## Installation

### Quick install

From GitHub (recommended):

```bash
curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent-ai/main/install.sh | bash
```

Or clone and run from project root:

```bash
git clone https://github.com/josephpaul/opsagent-ai.git && cd opsagent-ai && ./install.sh
```

The script will:

1. Build the Go CLI.
2. Copy the binary to `/usr/local/bin` (or prompt for `sudo` if needed).
3. Install Python dependencies for the AI agent (`pip install -r ai-agent/requirements.txt`).

### Manual setup

**CLI**

```bash
cd cli && go build -o opsagent-ai . && sudo cp opsagent-ai /usr/local/bin/
```

**Server Agent** – no install step; run with `go run main.go` from `server-agent/`.

**AI Agent**

```bash
cd ai-agent
python3 -m venv .venv
source .venv/bin/activate   # or .venv\Scripts\activate on Windows
pip install -r requirements.txt
```

Set your API key for the provider you use (in `ai-agent/.env` or environment):

```bash
# For Gemini (default)
export GOOGLE_API_KEY=your_key

# For Anthropic (Claude)
export ANTHROPIC_API_KEY=your_key

# For OpenAI
export OPENAI_API_KEY=your_key
```

Optionally set the default provider and model:

```bash
export AI_PROVIDER=gemini    # or anthropic | openai
export AI_MODEL=gemini-2.5-flash   # optional model override
```

## How to run the agents

1. **Server Agent** (on the machine you want to diagnose):

   ```bash
   cd server-agent && go run main.go
   ```
   Listens on **port 8080**.

2. **AI Agent** (same or another machine; must reach server agent on 8080):

   ```bash
   cd ai-agent && python3 -m uvicorn main:app --host 0.0.0.0 --port 9000
   ```
   Listens on **port 9000**. Set `SERVER_AGENT_URL=http://localhost:8080` if the server agent is elsewhere.

3. **CLI** (from anywhere that can reach the AI agent on 9000):

   ```bash
   opsagent-ai "why is my server slow"
   opsagent-ai "check cpu"
   opsagent-ai "check disk usage"
   ```

   Custom AI agent URL or provider:

   ```bash
   opsagent-ai --agent-url http://my-host:9000 "check memory"
   opsagent-ai --provider openai "check cpu"
   opsagent-ai --provider anthropic --model claude-3-5-sonnet-20241022 "why is my server slow"
   ```

## Choosing the AI provider

The AI agent supports **Gemini** (Google), **Anthropic** (Claude), and **OpenAI**. You can choose in two ways:

1. **Default (environment)** – In `ai-agent/.env` set `AI_PROVIDER=gemini` (or `anthropic` / `openai`) and the corresponding API key. The agent uses this by default for every request.

2. **Per request (CLI or API)** – Override per call:
   - **CLI:** `opsagent-ai --provider openai "check cpu"` or `opsagent-ai --provider anthropic --model claude-3-5-sonnet-20241022 "why is my server slow"`
   - **API:** POST `/query` with body `{"question": "...", "provider": "openai", "model": "gpt-4o"}` (both fields optional).

For Anthropic and OpenAI the agent uses LiteLLM; ensure `litellm` is installed (`pip install -r ai-agent/requirements.txt`).

## Example CLI usage

```bash
# Natural language (uses default provider from env)
opsagent-ai "why is my server slow"
opsagent-ai "what is using my disk"

# Direct checks
opsagent-ai "check cpu"
opsagent-ai "check memory"
opsagent-ai "check disk usage"
opsagent-ai "check top processes"

# Choose provider and optional model
opsagent-ai --provider openai "check cpu"
opsagent-ai --provider anthropic "why is my server slow"
opsagent-ai --provider gemini --model gemini-2.5-flash "check disk usage"
```

The CLI prints a **Diagnosis** line and **Details** from the AI agent.

## API reference

### AI Agent (port 9000)

- **POST /query**  
  Body: `{"question": "why is my server slow", "provider": "gemini|anthropic|openai", "model": "optional model name"}`  
  Response: `{"diagnosis": "...", "details": "..."}`

- **GET /providers**  
  Returns supported providers, default models, and env variable hints.

- **GET /health**  
  Returns `{"status": "ok"}`.

### Server Agent (port 8080)

- **GET /cpu** – `top -bn1` → `cpu_usage`, `top_process`, `raw_sample`
- **GET /memory** – `free -m` → `total_mb`, `used_mb`, `usage_percent`, etc.
- **GET /disk** – `df -h` → `filesystems` (size, used, avail, use_percent, mounted_on)
- **GET /processes** – `ps aux --sort=-%cpu | head` → `processes` (user, pid, cpu, mem, command)
- **GET /health** – `{"status": "ok"}`

## Requirements

- **Go** 1.21+ (CLI and Server Agent)
- **Python** 3.10+ (AI Agent)
- **API key** for at least one provider: Gemini (`GOOGLE_API_KEY`), Anthropic (`ANTHROPIC_API_KEY`), or OpenAI (`OPENAI_API_KEY`)
- **Linux** for the Server Agent (uses `top`, `free`, `df`, `ps`)

## License

Open source. Use and modify as needed.

## Contributing

Contributions are welcome. Please open an issue or PR on the repository.
