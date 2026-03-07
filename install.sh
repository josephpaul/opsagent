#!/usr/bin/env bash
# install.sh - Build and install opsagent (single Go binary).
# Usage: curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent/main/install.sh | bash
# Or: ./install.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "==> OpsAgent-AI Installer"
echo ""

# --- Build ---
echo "==> Building opsagent (Go)..."
if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed. Please install Go and try again."
  exit 1
fi
go build -o opsagent .
echo "    Build complete: ./opsagent"
echo ""

# --- Install binary ---
echo "==> Installing to /usr/local/bin..."
if [[ -w /usr/local/bin ]]; then
  cp opsagent /usr/local/bin/opsagent
  echo "    Installed: /usr/local/bin/opsagent"
else
  echo "    Skipping (no write permission to /usr/local/bin). Copy manually:"
  echo "    sudo cp $SCRIPT_DIR/opsagent /usr/local/bin/"
fi
echo ""

echo ""
echo "==> Installation complete!"
echo ""
echo "Quick start:"
echo "  1. Configure your API key:"
echo "     opsagent config set"
echo ""
echo "  2. Run from anywhere:"
echo "     opsagent \"why is my server slow\""
echo "     opsagent \"check cpu\""
echo "     opsagent \"check disk usage\""
echo ""
