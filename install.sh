#!/usr/bin/env bash
# install.sh - Build and install opsagent-ai (single Go binary).
# Usage: curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent-ai/main/install.sh | bash
# Or: ./install.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> OpsAgent-AI Installer"
echo ""

echo "==> Building opsagent-ai (Go)..."
if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed. Please install Go and try again."
  exit 1
fi
go build -o opsagent-ai .
echo "    Build complete: ./opsagent-ai"
echo ""

echo "==> Installing to /usr/local/bin..."
if [[ -w /usr/local/bin ]]; then
  cp opsagent-ai /usr/local/bin/opsagent-ai
  echo "    Installed: /usr/local/bin/opsagent-ai"
else
  echo "    Skipping (no write permission to /usr/local/bin). Copy manually:"
  echo "    sudo cp $SCRIPT_DIR/opsagent-ai /usr/local/bin/"
fi

echo ""
echo "==> Installation complete."
echo ""
echo "Set your Gemini API key, then run:"
echo "  export GOOGLE_API_KEY=your_key"
echo "  opsagent-ai \"why is my server slow\""
echo "  opsagent-ai \"check cpu\""
echo "  opsagent-ai \"check disk usage\""
echo ""
