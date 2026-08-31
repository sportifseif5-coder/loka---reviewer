#!/usr/bin/env bash
# Run the full test suite with module networking disabled. Tests are designed
# to need no network and no keys; this script makes that property explicit
# rather than assumed. GOPROXY=off makes any module fetch fail, and, when the
# host allows it, a network namespace also blocks sockets.
set -euo pipefail

export GOTOOLCHAIN=local
export GOPROXY=off
export GOSUMDB=off

run_offline() {
    echo "running tests with module network disabled"
    go test ./... -count=1
}

run_netns() {
    echo "running tests with networking disabled (network namespace)"
    unshare -n go test ./... -count=1
}

run_netns_sudo() {
    echo "running tests with networking disabled (network namespace via sudo)"
    # sudo resets PATH; carry the runner's PATH and toolchain settings over.
    sudo env "PATH=$PATH" GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off unshare -n go test ./... -count=1
}

if unshare -n true 2>/dev/null; then
    run_netns
elif sudo -n true 2>/dev/null && sudo unshare -n true 2>/dev/null; then
    run_netns_sudo
else
    echo "warning: cannot disable networking here; module network stays disabled (GOPROXY=off)"
    run_offline
fi
