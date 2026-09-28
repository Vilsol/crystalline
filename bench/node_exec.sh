#!/usr/bin/env bash
#
# Runs a js/wasm test binary under node.
#
# The environment is cleared first: the Go wasm runtime caps the combined size
# of argv and the environment, and an inherited one usually exceeds it.
#
# count.cjs is preloaded so the benchmarks can report crossings and js.FuncOf
# calls alongside the time.

set -euo pipefail

runner="$(go env GOROOT)/lib/wasm/wasm_exec_node.js"

exec env -i PATH="$PATH" node --require "$(dirname "$0")/count.cjs" "$runner" "$@"
