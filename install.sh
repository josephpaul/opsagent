#!/usr/bin/env bash
# install.sh - Clone (if needed), build, and install opsagent.
# Usage: curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent/main/install.sh | bash
# Or from the cloned repo: ./install.sh

set -e

REPO="https://github.com/josephpaul/opsagent.git"
BINARY="opsagent"

echo "==> OpsAgent Installer"
echo ""

if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed. Please install Go 1.21+ and try again."
  exit 1
fi

if ! command -v git &>/dev/null; then
  echo "Error: git is not installed."
  exit 1
fi

# If go.mod exists in the current directory we're already inside the repo.
# Otherwise clone into a temp directory.
CLEANUP=""
if [[ -f go.mod ]]; then
  SRC_DIR="$(pwd)"
elif [[ -f "$(dirname "${BASH_SOURCE[0]:-$0}")/go.mod" ]]; then
  SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
else
  TMPDIR="$(mktemp -d)"
  CLEANUP="$TMPDIR"
  echo "==> Cloning $REPO ..."
  git clone --depth 1 "$REPO" "$TMPDIR/opsagent"
  SRC_DIR="$TMPDIR/opsagent"
fi

cd "$SRC_DIR"

echo "==> Building $BINARY (Go)..."
go build -o "$BINARY" .
echo "    Build complete: $SRC_DIR/$BINARY"
echo ""

echo "==> Installing to /usr/local/bin..."
if [[ -w /usr/local/bin ]]; then
  cp "$BINARY" /usr/local/bin/"$BINARY"
  echo "    Installed: /usr/local/bin/$BINARY"
else
  sudo cp "$BINARY" /usr/local/bin/"$BINARY"
  echo "    Installed: /usr/local/bin/$BINARY (via sudo)"
fi
echo ""

if [[ -n "$CLEANUP" ]]; then
  rm -rf "$CLEANUP"
fi

echo "==> Installation complete!"
echo ""
echo "Quick start:"
echo "  1. Configure your API key:"
echo "     $BINARY config set"
echo ""
echo "  2. Run from anywhere:"
echo "     $BINARY \"why is my server slow\""
echo "     $BINARY \"check cpu\""
echo "     $BINARY \"check disk usage\""
echo ""

# --- Optional: background monitoring ---
echo "==> Optional: Background Monitoring"
echo "    OpsAgent can run as a background service that monitors CPU and RAM"
echo "    and triggers an AI diagnosis when thresholds are breached."
echo ""
read -r -p "    Enable background monitoring? [y/N]: " ENABLE_MONITOR
if [[ "$ENABLE_MONITOR" =~ ^[Yy]$ ]]; then
  read -r -p "    Polling interval (e.g. 30s, 1m, 5m) [60s]: " MON_INTERVAL
  MON_INTERVAL="${MON_INTERVAL:-60s}"
  read -r -p "    CPU alert threshold % [90]: " MON_CPU
  MON_CPU="${MON_CPU:-90}"
  read -r -p "    RAM alert threshold % [85]: " MON_RAM
  MON_RAM="${MON_RAM:-85}"

  echo ""
  echo "==> Installing background monitor service..."
  "$BINARY" monitor install \
    --interval "$MON_INTERVAL" \
    --cpu-threshold "$MON_CPU" \
    --ram-threshold "$MON_RAM"
  echo ""
  echo "    Monitor installed! Manage with:"
  echo "      $BINARY monitor status"
  echo "      $BINARY monitor stop"
  echo "      $BINARY monitor uninstall"
else
  echo ""
  echo "    Skipped. You can enable it later with:"
  echo "      $BINARY monitor install --interval 60s --cpu-threshold 90 --ram-threshold 85"
fi
echo ""
