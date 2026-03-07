#!/usr/bin/env bash
# install.sh - Build and install opsagent-ai CLI, and prepare Python agent environment.
# Usage: curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent-ai/main/install.sh | bash
# Or: ./install.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> OpsAgent-AI Installer"
echo ""

# 1. Build Go CLI
echo "==> Building CLI (Go)..."
if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed. Please install Go and try again."
  exit 1
fi
cd cli
go build -o opsagent-ai .
cd "$SCRIPT_DIR"

# 2. Move CLI to /usr/local/bin (optional; may require sudo)
echo "==> Installing CLI to /usr/local/bin..."
if [[ -w /usr/local/bin ]]; then
  cp cli/opsagent-ai /usr/local/bin/opsagent-ai
  echo "    Installed: /usr/local/bin/opsagent-ai"
else
  echo "    Skipping (no write permission to /usr/local/bin). Copy manually:"
  echo "    sudo cp $SCRIPT_DIR/cli/opsagent-ai /usr/local/bin/"
fi

# 3. Install Python dependencies for AI agent
echo "==> Installing Python dependencies (ai-agent)..."
if ! command -v python3 &>/dev/null && ! command -v python &>/dev/null; then
  echo "Warning: Python not found. Install Python 3.10+ and run: pip install -r ai-agent/requirements.txt"
else
  PIP="python3 -m pip"
  if ! python3 -m pip --version &>/dev/null; then
    PIP="python -m pip"
  fi
  $PIP install -r ai-agent/requirements.txt -q
  echo "    Python dependencies installed."
fi

echo ""
echo "==> Installation complete."
echo ""
echo "To run the system:"
echo ""
echo "  1. Start the Server Agent (diagnostics API on port 8080):"
echo "     cd $SCRIPT_DIR/server-agent && go run main.go"
echo ""
echo "  2. Set your Google API key for the AI agent (Gemini):"
echo "     export GOOGLE_API_KEY=your_key"
echo "     # Or create ai-agent/.env with: GOOGLE_API_KEY=your_key"
echo ""
echo "  3. Start the AI Agent (query API on port 9000):"
echo "     cd $SCRIPT_DIR/ai-agent && python3 -m uvicorn main:app --host 0.0.0.0 --port 9000"
echo ""
echo "  4. Use the CLI:"
echo "     opsagent-ai \"why is my server slow\""
echo "     opsagent-ai \"check cpu\""
echo "     opsagent-ai \"check disk usage\""
echo ""
