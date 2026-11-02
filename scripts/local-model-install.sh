#!/usr/bin/env bash
# Install the Ollama binary only. Never downloads model weights.
# Phase 1.9 still requires a later `make local-model-pull` (or ollama pull).
set -euo pipefail

if command -v ollama >/dev/null 2>&1; then
    echo "ollama already installed: $(command -v ollama)"
    ollama --version 2>/dev/null || true
    echo "weights are not installed. run: make local-model-status"
    exit 0
fi

arch="$(uname -m)"
case "$arch" in
    x86_64) file="ollama-linux-amd64.tgz" ;;
    aarch64|arm64) file="ollama-linux-arm64.tgz" ;;
    *)
        echo "unsupported architecture: $arch" >&2
        exit 1
        ;;
esac

url="https://github.com/ollama/ollama/releases/latest/download/${file}"
tmp="$(mktemp -d)"
echo "installing ollama binary from $url (no model pull, no systemd)"
curl -fsSL --max-time 180 "$url" -o "$tmp/$file"
tar -C /usr/local -xzf "$tmp/$file"

if ! command -v ollama >/dev/null 2>&1; then
    echo "ollama not on PATH after install" >&2
    exit 1
fi

echo "installed: $(command -v ollama)"
ollama --version 2>/dev/null || true
echo "weights are not installed. run: make local-model-start && make local-model-status"
