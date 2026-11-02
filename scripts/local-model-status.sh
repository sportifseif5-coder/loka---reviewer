#!/usr/bin/env bash
# Report local-model runtime readiness. Exit codes:
#   0  server up and the configured model is present
#   2  server up, waiting for weights (Phase 1.9 still open)
#   1  runtime missing or not listening
# Never pulls a model. Never ticks Phase 1.9.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${OLLAMA_HOST:-127.0.0.1:11434}"
PING_URL="http://${HOST}/v1/models"
MODEL="${LOKA_LOCAL_MODEL:-qwen2.5-coder:7b}"

cfg="$ROOT/.loka/review.yaml"
if [ -f "$cfg" ]; then
    from_cfg="$(python3 - "$cfg" <<'PY'
import sys
path = sys.argv[1]
model = ""
try:
    import yaml
except ImportError:
    yaml = None
if yaml is not None:
    with open(path) as f:
        data = yaml.safe_load(f) or {}
    providers = data.get("providers") or []
    if providers:
        model = (providers[0] or {}).get("model") or ""
else:
    want = False
    with open(path) as f:
        for line in f:
            s = line.strip()
            if s.startswith("model:"):
                model = s.split(":", 1)[1].strip().strip("'\"")
                break
print(model)
PY
)"
    if [ -n "$from_cfg" ]; then
        MODEL="$from_cfg"
    fi
fi

echo "host=$HOST"
echo "model=$MODEL"

if ! command -v ollama >/dev/null 2>&1; then
    echo "status=runtime-missing"
    echo "install with: make local-model-install"
    exit 1
fi
echo "binary=$(command -v ollama)"

code="$(curl -s -o /tmp/loka-ollama-models.json -w '%{http_code}' --max-time 3 "$PING_URL" 2>/dev/null || true)"
if [ -z "$code" ] || [ "$code" = "000" ] || [ "$code" -ge 500 ]; then
    echo "status=runtime-down http=${code:-none}"
    echo "start with: make local-model-start"
    exit 1
fi
echo "http=$code"

python3 - "$MODEL" /tmp/loka-ollama-models.json <<'PY'
import json, sys
want = sys.argv[1]
path = sys.argv[2]
try:
    data = json.load(open(path))
except Exception as e:
    print("status=runtime-up-unreadable")
    print("error=%s" % e)
    sys.exit(2)
ids = []
for m in data.get("data") or data.get("models") or []:
    if isinstance(m, dict):
        ids.append(m.get("id") or m.get("name") or "")
print("models=%s" % (",".join(x for x in ids if x) or "(none)"))
names = set(ids)
# Ollama lists tags as name:tag and sometimes name:tag plus digest aliases.
have = want in names or any(x.split(":")[0] == want.split(":")[0] and want in x for x in names)
if have:
    print("status=ready")
    sys.exit(0)
print("status=waiting-for-weights")
print("next=make local-model-pull")
print("phase_1_9=still open until make dogfood shows llm:<model>")
sys.exit(2)
PY
