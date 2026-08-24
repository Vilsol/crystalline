#!/usr/bin/env bash
#
# Runs a js/wasm test binary under node.
#
# The environment is cleared first: the Go wasm runtime caps the combined size
# of argv and the environment, and an inherited one usually exceeds it.

set -euo pipefail

runner="$(go env GOROOT)/lib/wasm/wasm_exec_node.js"

exec env -i PATH="$PATH" node "$runner" "$@"
