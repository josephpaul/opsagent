#!/usr/bin/env bash
# install.sh - Download a pre-built binary or build from source, then install opsagent.
# Usage: curl -sSL https://raw.githubusercontent.com/josephpaul/opsagent/main/install.sh | bash
# Or from the cloned repo: ./install.sh

set -e

REPO="josephpaul/opsagent"
BINARY="opsagent"
INSTALL_DIR="/usr/local/bin"

echo "==> OpsAgent Installer"
echo ""

# --- Detect OS and architecture ---
detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"

  case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    mingw*|msys*|cygwin*) OS="windows" ;;
    *)
      echo "Unsupported OS: $OS"
      exit 1
      ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
      echo "Unsupported architecture: $ARCH"
      exit 1
      ;;
  esac
}

detect_platform

EXT=""
if [ "$OS" = "windows" ]; then
  EXT=".exe"
fi

ASSET_NAME="${BINARY}-${OS}-${ARCH}${EXT}"

# --- Try downloading pre-built binary from GitHub Releases ---
install_from_release() {
  echo "==> Detecting latest release..."

  LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"

  if command -v curl &>/dev/null; then
    RELEASE_JSON="$(curl -fsSL "$LATEST_URL" 2>/dev/null)" || return 1
  elif command -v wget &>/dev/null; then
    RELEASE_JSON="$(wget -qO- "$LATEST_URL" 2>/dev/null)" || return 1
  else
    return 1
  fi

  DOWNLOAD_URL="$(echo "$RELEASE_JSON" | grep -o "\"browser_download_url\": *\"[^\"]*${ASSET_NAME}\"" | head -1 | cut -d'"' -f4)"

  if [ -z "$DOWNLOAD_URL" ]; then
    echo "    No pre-built binary found for ${OS}/${ARCH}."
    return 1
  fi

  TAG="$(echo "$RELEASE_JSON" | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)"
  echo "    Found release: ${TAG}"
  echo "    Downloading ${ASSET_NAME}..."

  TMPDIR="$(mktemp -d)"
  TMPFILE="${TMPDIR}/${BINARY}${EXT}"

  if command -v curl &>/dev/null; then
    curl -fsSL -o "$TMPFILE" "$DOWNLOAD_URL" || { rm -rf "$TMPDIR"; return 1; }
  else
    wget -qO "$TMPFILE" "$DOWNLOAD_URL" || { rm -rf "$TMPDIR"; return 1; }
  fi

  chmod +x "$TMPFILE"

  echo "==> Installing to ${INSTALL_DIR}..."
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}${EXT}"
  else
    sudo mv "$TMPFILE" "${INSTALL_DIR}/${BINARY}${EXT}"
  fi

  rm -rf "$TMPDIR"
  echo "    Installed: ${INSTALL_DIR}/${BINARY}${EXT}"
  return 0
}

# --- Fallback: build from source ---
install_from_source() {
  echo "==> Building from source..."

  if ! command -v go &>/dev/null; then
    echo ""
    echo "Error: Go is not installed and no pre-built binary is available."
    echo ""
    echo "Options:"
    echo "  1. Install Go (https://go.dev/dl/) and re-run this script"
    echo "  2. Download a pre-built binary from https://github.com/${REPO}/releases"
    exit 1
  fi

  if ! command -v git &>/dev/null; then
    echo "Error: git is not installed."
    exit 1
  fi

  CLEANUP=""
  if [ -f go.mod ]; then
    SRC_DIR="$(pwd)"
  elif [ -f "$(dirname "${BASH_SOURCE[0]:-$0}")/go.mod" ]; then
    SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
  else
    TMPDIR="$(mktemp -d)"
    CLEANUP="$TMPDIR"
    echo "    Cloning https://github.com/${REPO}.git ..."
    git clone --depth 1 "https://github.com/${REPO}.git" "$TMPDIR/opsagent"
    SRC_DIR="$TMPDIR/opsagent"
  fi

  cd "$SRC_DIR"

  echo "    Compiling..."
  go build -o "${BINARY}${EXT}" .
  echo "    Build complete."

  echo "==> Installing to ${INSTALL_DIR}..."
  if [ -w "$INSTALL_DIR" ]; then
    cp "${BINARY}${EXT}" "${INSTALL_DIR}/${BINARY}${EXT}"
  else
    sudo cp "${BINARY}${EXT}" "${INSTALL_DIR}/${BINARY}${EXT}"
  fi
  echo "    Installed: ${INSTALL_DIR}/${BINARY}${EXT}"

  if [ -n "$CLEANUP" ]; then
    rm -rf "$CLEANUP"
  fi
}

# --- Main: try binary download first, fall back to source ---
if install_from_release; then
  echo ""
else
  echo ""
  install_from_source
  echo ""
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
echo "  3. Update later:"
echo "     $BINARY update"
echo ""

# --- Optional: background monitoring ---
if [ -t 0 ]; then
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
fi
