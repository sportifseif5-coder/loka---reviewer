#!/usr/bin/env bash
# Phase 1.9 dogfood: review this repository offline and print wall-clock
# plus a compact findings summary. A local model (Ollama / llama.cpp) is
# used when reachable; otherwise the deterministic baseline still ships.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DB="${LOKA_DB:-${TMPDIR:-/tmp}/loka-dogfood.db}"
OUT="${LOKA_OUT:-${TMPDIR:-/tmp}/loka-dogfood.json}"

cd "$ROOT"
go build -o "${TMPDIR:-/tmp}/loka" ./cmd/loka

start="$(date +%s%N)"
set +e
"${TMPDIR:-/tmp}/loka" review -repo "$ROOT" -db "$DB" -json >"$OUT" 2>"${OUT}.err"
status=$?
set -e
end="$(date +%s%N)"
elapsed_ms=$(( (end - start) / 1000000 ))

echo "exit=$status elapsed_ms=$elapsed_ms db=$DB out=$OUT"
if [ -s "${OUT}.err" ]; then
    echo "stderr:"
    cat "${OUT}.err"
fi
python3 - "$OUT" "$elapsed_ms" <<'PY'
import json, sys
path, elapsed = sys.argv[1], int(sys.argv[2])
with open(path) as f:
    res = json.load(f)
findings = res.get("findings") or []
print("review_id=%s mode=%s risk=%s findings=%d analyzers=%s" % (
    res.get("id"), res.get("mode"), (res.get("summary") or {}).get("risk"),
    len(findings), ",".join(res.get("analyzers_run") or [])))
walk = (res.get("summary") or {}).get("walkthrough") or ""
if walk:
    print(walk)
for d in res.get("degradations") or []:
    print("degradation: %s" % d)
for w in res.get("warnings") or []:
    print("warning: %s" % w)
print("--- findings ---")
for f in findings:
    loc = f.get("location") or {}
    print("%s\t%s\t%s\t%s:%s\t%s" % (
        f.get("severity"), f.get("source"), f.get("rule_id") or f.get("category"),
        loc.get("file"), loc.get("line_start"), f.get("message", "").split("\n", 1)[0]))
print("elapsed_ms=%d budget_ms=60000" % elapsed)
if elapsed > 60000:
    print("OVER_BUDGET")
    sys.exit(2)
PY
