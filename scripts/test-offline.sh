#!/usr/bin/env bash
# Run the full test suite with networking disabled, when the host allows it.
# Tests are designed to need no network and no keys; this script makes that
# property explicit rather than assumed.
set -euo pipefail

run_offline() {
    echo "running tests with networking disabled"
    # Never attempt a toolchain auto-download inside the isolated namespace.
    GOTOOLCHAIN=local unshare -n go test ./... -count=1
}

# Try a user namespace with networking unshared first (no privileges needed
# on most modern kernels), then sudo, then fall back to a plain run.
if unshare -n true 2>/dev/null; then
    run_offline
elif sudo -n true 2>/dev/null && sudo unshare -n true 2>/dev/null; then
    echo "running tests with networking disabled (via sudo)"
    sudo -E GOTOOLCHAIN=local unshare -n go test ./... -count=1
else
    echo "warning: cannot disable networking here; tests are offline by design"
    go test ./... -count=1
fi
