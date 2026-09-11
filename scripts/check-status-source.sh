#!/usr/bin/env bash
# Enforce a single source of truth for work status: checklist status markers
# ([x] done, [~] partial, [ ] not started) may live only in
# docs/PROGRESS.md. Scope and exit-criteria definitions live in
# docs/ROADMAP.md and the narrative lives in docs/SESSION_NOTES.md, neither
# of which may carry status markers. This keeps the checklist from drifting
# out of sync with prose copies elsewhere.
set -euo pipefail

offenders="$(
    grep -rn --include='*.md' --exclude-dir=.git --exclude-dir=node_modules \
        -E '^[[:space:]]*- \[[ x~]\]' . \
        | grep -v '^\./docs/PROGRESS\.md:' || true
)"

if [ -n "$offenders" ]; then
    echo "error: status markers found outside docs/PROGRESS.md:" >&2
    echo "$offenders" >&2
    echo "error: track status only in docs/PROGRESS.md (AGENTS.md section 4)." >&2
    exit 1
fi

echo "status source check: only docs/PROGRESS.md carries status markers"
