#!/usr/bin/env bash
# Start the local Ollama runtime on 127.0.0.1:11434 if it is not already up.
# Does not pull model weights.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${OLLAMA_HOST:-127.0.0.1:11434}"
export OLLAMA_HOST="$HOST"
LOG="${OLLAMA_LOG:-${TMPDIR:-/tmp}/loka-ollama.log}"
PIDFILE="${OLLAMA_PIDFILE:-${TMPDIR:-/tmp}/loka-ollama.pid}"
PING_URL="http://${HOST}/v1/models"

if ! command -v ollama >/dev/null 2>&1; then
    echo "ollama not installed. run: make local-model-install" >&2
    exit 1
fi

code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$PING_URL" 2>/dev/null || true)"
if [ -n "$code" ] && [ "$code" != "000" ] && [ "$code" -ge 200 ] && [ "$code" -lt 500 ]; then
    echo "ollama already listening on $HOST (HTTP $code)"
    exit 0
fi

echo "starting ollama serve on $HOST"
nohup ollama serve >>"$LOG" 2>&1 &
echo $! >"$PIDFILE"

for _ in 1 2 3 4 5 6 7 8 9 10; do
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$PING_URL" 2>/dev/null || true)"
    if [ -n "$code" ] && [ "$code" != "000" ] && [ "$code" -ge 200 ] && [ "$code" -lt 500 ]; then
        echo "ollama ready on $HOST (HTTP $code) pid=$(cat "$PIDFILE") log=$LOG"
        echo "model weights are not installed. run: make local-model-status"
        exit 0
    fi
    sleep 1
done

echo "ollama did not answer $PING_URL within 10s; see $LOG" >&2
exit 1
