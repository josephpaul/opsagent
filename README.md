# OpsAgent-AI

A single-binary CLI that diagnoses the **current machine** using natural language. Ask questions like *"why is my server slow"* or *"check disk usage"* and get an AI-powered diagnosis backed by real system metrics (CPU, memory, disk, processes).

## Architecture

OpsAgent-AI is one Go application using [Google ADK for Go](https://github.com/google/adk-go):

- **CLI (Cobra)** – Parses your query and invokes the agent in-process.
- **ADK Agent (Gemini, OpenAI, or Anthropic)** – Decides which diagnostic tools to run and turns their output into a human-readable diagnosis.
- **Diagnostic tools** – Four in-process tools that gather CPU, memory, disk, and process data using OS-native commands (Linux, macOS, and Windows).

```
┌──────────────────────────────────────────────────────────────┐
│  opsagent (single binary)                                    │
│                                                              │
│  ┌──────────┐   ┌─────────────┐   ┌─────────────────┐       │
│  │  CLI     │──►│  ADK Agent  │──►│ check_cpu       │       │
│  │ (Cobra)  │   │  (LLM)      │   │ check_memory    │       │
│  │          │◄──│             │◄──│ check_disk      │       │
│  └──────────┘   └──────┬──────┘   │ check_processes │       │
│  ┌──────────┐          │          └─────────────────┘       │
│  │ Telegram │◄─────────┘                                    │
│  │  Bot     │  (webhook / poll)                             │
│  └──────────┘          │                                    │
│  ┌──────────┐   ┌──────┴──────┐                             │
│  │ Monitor  │   │ SQLite      │  sessions.db                │
│  │          │──►│ Sessions    │  (conversation memory)      │
│  └──────────┘   └─────────────┘                             │
└──────────────────────────────────────────────────────────────┘
```

Diagnostics run on the machine where you execute the binary. No separate server or Python process.

## Requirements

- **API key** for your chosen provider
- **Linux, macOS, or Windows**
  - Linux: uses `top`, `free`, `df`, `ps`
  - macOS: uses `top`, `vm_stat`, `df`, `ps`
  - Windows: uses PowerShell (`Get-CimInstance`, `Get-Process`)

No Go installation required — pre-built binaries are available for all platforms.

## Installation

### Quick install (recommended)

Downloads a pre-built binary — no Go or build tools needed:

```bash
curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent/main/install.sh | bash
```

The script auto-detects your OS and architecture, downloads the right binary from GitHub Releases, and installs it to `/usr/local/bin`. If no pre-built binary is available, it falls back to building from source (requires Go).

### Download manually

Grab a binary from the [Releases page](https://github.com/josephpaul/opsagent/releases):

| Platform | Binary |
|---|---|
| Linux (x86_64) | `opsagent-linux-amd64` |
| Linux (ARM64) | `opsagent-linux-arm64` |
| macOS (Intel) | `opsagent-darwin-amd64` |
| macOS (Apple Silicon) | `opsagent-darwin-arm64` |
| Windows (x86_64) | `opsagent-windows-amd64.exe` |

```bash
chmod +x opsagent-*
sudo mv opsagent-* /usr/local/bin/opsagent
```

### Build from source

Requires Go 1.21+:

```bash
git clone https://github.com/josephpaul/opsagent.git && cd opsagent
go build -o opsagent .
sudo cp opsagent /usr/local/bin/
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
| `TELEGRAM_BOT_TOKEN` | Telegram bot token (from @BotFather) |
| `TELEGRAM_USER_ID` | **Required for Telegram.** Your numeric Telegram user ID. Only this user can interact with the bot. |
| `TELEGRAM_CHAT_ID` | Destination chat ID for monitor alerts |
| `TELEGRAM_WEBHOOK_SECRET` | Webhook request verification secret |
| `LOGIN_NOTIFY_ENABLED` | Enable or disable PAM-triggered login alerts (`true` by default) |
| `LOGIN_NOTIFY_HOSTNAME_LABEL` | Optional hostname override shown in login alerts |
| `LOGIN_NOTIFY_TIMEZONE` | Optional IANA timezone for login alert timestamps (for example `Asia/Kolkata`) |
| `LOGIN_NOTIFY_TIME_FORMAT` | Optional Go `time.Format` layout for login alert timestamps |
| `MONITOR_INTERVAL` | Default monitor interval for `monitor run` / `monitor install` |
| `MONITOR_CPU_THRESHOLD` | Default monitor CPU threshold (%) for `monitor run` / `monitor install` |
| `MONITOR_RAM_THRESHOLD` | Default monitor RAM threshold (%) for `monitor run` / `monitor install` |
| `MONITOR_LOG_PATH` | Default monitor log path for `monitor run` / `monitor install` |

All of these can be set from the CLI, so you never need a `.env` file.

## Conversation Memory

OpsAgent remembers previous interactions so the AI agent has context across multiple queries. Memory is stored locally in a SQLite database alongside your config.

| OS | Database path |
|---|---|
| macOS | `~/.config/opsagent/sessions.db` |
| Linux | `~/.config/opsagent/sessions.db` |
| Windows | `%APPDATA%\opsagent\sessions.db` |

### How it works

- **CLI**: Consecutive `opsagent "..."` commands share a session, so the agent remembers what you asked before. Use `--no-memory` to start fresh for a single query.
- **Telegram bot**: Each chat gets its own session. The bot remembers the conversation thread per user/group.
- **Monitor**: Background alerts are stateless — each diagnosis starts fresh.

The last 20 messages are loaded into the LLM context to keep token usage bounded. Older messages stay in the database but aren't sent to the model.

### Clearing history

```bash
opsagent clear                # Clear CLI conversation history
opsagent clear --user tg-123  # Clear a specific Telegram chat's history
opsagent clear --all          # Clear all stored sessions
```

In Telegram, send `/clear` to the bot to reset your conversation.

### Disabling memory

```bash
opsagent --no-memory "check cpu"
```

This uses a fresh in-memory session for that single query without touching the database.

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
opsagent monitor set --interval 30s --cpu-threshold 85 --ram-threshold 80
opsagent monitor status      # Check if running
opsagent monitor stop        # Stop the service
opsagent monitor start       # Start the service
opsagent monitor uninstall   # Remove the service entirely
```

### Save default monitor settings in config

You can persist monitor defaults in `config.yaml` so future `monitor run` and
`monitor install` commands use them automatically when flags are omitted.

```bash
opsagent monitor set --interval 30s --cpu-threshold 85 --ram-threshold 80
opsagent monitor set --log /var/log/opsagent/monitor.log
opsagent monitor set --clear-log
```

Precedence for `monitor run` and `monitor install`:
1. Explicit CLI flags
2. Monitor defaults from config (`MONITOR_*`)
3. Built-in defaults (`60s`, `90`, `85`)

### Monitor flags

| Flag | Default | Description |
|---|---|---|
| `--interval` | `60s` | How often to poll (e.g. `30s`, `5m`, `1h`) |
| `--cpu-threshold` | `90` | CPU % that triggers an AI diagnosis |
| `--ram-threshold` | `85` | RAM % that triggers an AI diagnosis |
| `--log` | *(stdout only)* | Also write logs to this file |

The installer (`install.sh`) also offers to set up monitoring during installation.

### Telegram alerts for the monitor

Send threshold alerts and AI diagnoses to Telegram:

```bash
opsagent monitor run --telegram-token <BOT_TOKEN> --telegram-chat-id <CHAT_ID>
```

Or save the tokens so every monitor run uses them:

```bash
opsagent config set-key TELEGRAM_BOT_TOKEN <token>
opsagent config set-key TELEGRAM_CHAT_ID <chat_id>
```

## Login Notifications

OpsAgent can send a Telegram alert every time PAM opens a successful login session. This keeps the notification logic inside `opsagent` while PAM acts as the event trigger.

### Configure Telegram destination

```bash
opsagent config set-key TELEGRAM_BOT_TOKEN <token>
opsagent config set-key TELEGRAM_CHAT_ID <chat_id>
opsagent config set-key LOGIN_NOTIFY_ENABLED true
```

Optional hostname label override:

```bash
opsagent config set-key LOGIN_NOTIFY_HOSTNAME_LABEL batchai
```

Optional time settings:

```bash
opsagent config set-key LOGIN_NOTIFY_TIMEZONE Asia/Kolkata
opsagent config set-key LOGIN_NOTIFY_TIME_FORMAT "3:04pm 2 Jan 06"
```

`LOGIN_NOTIFY_TIME_FORMAT` uses Go's `time.Format` layout syntax, not `strftime`. If unset, OpsAgent uses `2006-01-02 15:04:05 MST`.

### Install the PAM hook

```bash
sudo opsagent login install-pam
opsagent login status
```

`opsagent login install-pam` automatically adds the OpsAgent PAM hook to the common PAM files when they exist:

- `/etc/pam.d/sshd`
- `/etc/pam.d/login`

It is idempotent and creates a backup next to each edited file as `*.opsagent.bak`.

If you only want to print the snippet without editing files:

```bash
opsagent login install-pam --print-only
```

Use an `optional` PAM hook so login is never blocked if notification delivery fails.

### PAM snippet

The installed line looks like this:

```pam
session optional pam_exec.so /usr/bin/env OPSAGENT_CONFIG_PATH=/home/your-user/.config/opsagent/config.yaml /usr/local/bin/opsagent login notify
```

The `OPSAGENT_CONFIG_PATH=...` part is important because PAM often runs `opsagent` as `root`; this ensures OpsAgent reads your user config instead of `/root/.config/opsagent/config.yaml`.

When PAM invokes `opsagent login notify`, OpsAgent reads `PAM_USER`, `PAM_RHOST`, `PAM_SERVICE`, and `PAM_TTY`, then sends a Telegram message like:

```text
Login detected
Event: Login
User: ubuntu
Source: 203.0.113.10
Service: sshd
TTY: ssh
Host: batchai
Time: 2026-03-14 19:22:10 UTC
```

`Event` is derived from PAM:
- `open_session` -> `Login`
- `close_session` -> `Logout`

Keep an existing admin session open while testing PAM changes.

## Telegram Bot

Run OpsAgent as a Telegram bot so you can send diagnostic queries from your phone or any Telegram client.

### Setup

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot`, follow the prompts, and copy the **bot token**
3. Save the token:
   ```bash
   opsagent config set-key TELEGRAM_BOT_TOKEN <token>
   ```
4. Save your Telegram user ID (get it from [@userinfobot](https://t.me/userinfobot)):
   ```bash
   opsagent config set-key TELEGRAM_USER_ID <your_numeric_id>
   ```

`TELEGRAM_USER_ID` is **required**. Without it, no Telegram command will start. Only messages from this user ID get a response — all others are silently ignored.

### Webhook mode (recommended for production)

Telegram pushes messages to your server — low latency, no polling overhead. Requires a public URL (use a reverse proxy like nginx or Cloudflare Tunnel).

```bash
opsagent telegram webhook --token <BOT_TOKEN> --webhook-url https://your-server.com
```

| Flag | Default | Description |
|---|---|---|
| `--token` | env `TELEGRAM_BOT_TOKEN` | Bot token from BotFather |
| `--webhook-url` | *(required)* | Public HTTPS URL for your server |
| `--port` | `8443` | Local port for the HTTP listener |
| `--user-id` | env `TELEGRAM_USER_ID` | **Required.** Your Telegram user ID — only this user gets responses |
| `--webhook-secret` | env `TELEGRAM_WEBHOOK_SECRET` | Secret for verifying requests are from Telegram (auto-generated if empty) |

The webhook endpoint is `POST /webhook/telegram` on the configured port. Every incoming request is verified using the `X-Telegram-Bot-Api-Secret-Token` header. You can set a persistent secret via config:

```bash
opsagent config set-key TELEGRAM_WEBHOOK_SECRET my-secret-value
```

If not set, a random secret is generated on each startup.

### Polling mode (simpler, no public IP needed)

OpsAgent fetches messages from Telegram using long polling. Great for development or servers behind NAT.

```bash
opsagent telegram poll --token <BOT_TOKEN>
```

### Bot commands

Once the bot is running, send messages in Telegram:

| Message | What it does |
|---|---|
| `/start` | Welcome message with usage hints |
| `/status` | Confirms the bot is running |
| `/clear` | Clears conversation history for your chat |
| Any text | Runs an AI-powered server diagnosis and replies |

### Install as a background service

Run the Telegram bot as a persistent service that auto-starts on boot:

```bash
# Webhook mode (production)
opsagent telegram install --mode webhook --webhook-url https://your-server.com

# Polling mode (simpler)
opsagent telegram install --mode poll
```

Manage the service:

```bash
opsagent telegram status      # Check if running
opsagent telegram stop        # Stop the service
opsagent telegram start       # Start the service
opsagent telegram uninstall   # Remove the service entirely
```

This creates a **systemd** user service on Linux or a **launchd** agent on macOS.

### Restricting access

`TELEGRAM_USER_ID` is mandatory for all Telegram features. Only messages from the configured user ID receive a response — all others are silently dropped.

To find your Telegram user ID:
- Message [@userinfobot](https://t.me/userinfobot) on Telegram — it replies with your numeric ID
- Or use the Telegram Bot API: `https://api.telegram.org/bot<TOKEN>/getUpdates` after sending a message to your bot, and look for `message.from.id`

```bash
opsagent config set-key TELEGRAM_USER_ID 123456789
```

## Updating

### Self-update

```bash
opsagent update          # Download and install the latest release
opsagent update --check  # Check for updates without installing
```

The update command checks GitHub Releases for a pre-built binary matching your OS and architecture. If no binary is available, it falls back to cloning the repo and building from source (requires Go).

### Manual update

```bash
curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent/main/install.sh | bash
```

Or pull and rebuild:

```bash
cd opsagent && git pull && go build -o opsagent . && sudo cp opsagent /usr/local/bin/
```

## License

Open source. Use and modify as needed.

## Contributing

Contributions are welcome. Please open an issue or PR on the repository.
