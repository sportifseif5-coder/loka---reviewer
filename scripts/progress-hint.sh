#!/usr/bin/env bash
# Advisory only - never fails the build. Warns when the working tree has Go
# source changes but no update to docs/PROGRESS.md, so the living checklist
# does not silently rot. Run via `make progress-hint` (also at the end of
# `make ci`).
set -euo pipefail

changed="$(git status --porcelain -uall 2>/dev/null | cut -c4-)"
if [ -z "$changed" ]; then
    exit 0
fi

go_changed="$(printf '%s\n' "$changed" | grep -E '\.go$' || true)"
progress_changed="$(printf '%s\n' "$changed" | grep -qx 'docs/PROGRESS.md' && echo yes || true)"

if [ -n "$go_changed" ] && [ -z "$progress_changed" ]; then
    echo "warning: Go files changed without a docs/PROGRESS.md update:"
    printf '%s\n' "$go_changed" | sed 's/^/  /'
    echo "warning: tick finished steps in docs/PROGRESS.md before committing."
fi
