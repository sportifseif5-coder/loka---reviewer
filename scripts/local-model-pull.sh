#!/usr/bin/env bash
# Download the configured local model into Ollama. Run this later when
# weights should be added. This is the only script that fetches a model.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${OLLAMA_HOST:-127.0.0.1:11434}"
export OLLAMA_HOST="$HOST"
MODEL="${LOKA_LOCAL_MODEL:-}"

if [ -z "$MODEL" ] && [ -f "$ROOT/.loka/review.yaml" ]; then
    MODEL="$(python3 - "$ROOT/.loka/review.yaml" <<'PY'
import sys
path = sys.argv[1]
model = "qwen2.5-coder:7b"
with open(path) as f:
    for line in f:
        s = line.strip()
        if s.startswith("model:"):
            model = s.split(":", 1)[1].strip().strip("'\"")
            break
print(model)
PY
)"
fi
MODEL="${MODEL:-qwen2.5-coder:7b}"

if ! command -v ollama >/dev/null 2>&1; then
    echo "ollama not installed. run: make local-model-install" >&2
    exit 1
fi

code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 2 "http://${HOST}/v1/models" || true)"
if [ -z "$code" ] || [ "$code" -ge 500 ]; then
    echo "ollama is not listening. run: make local-model-start" >&2
    exit 1
fi

echo "pulling $MODEL (this downloads weights)"
ollama pull "$MODEL"
echo "pulled $MODEL"
echo "next: make local-model-status && make dogfood"
