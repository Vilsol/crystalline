#!/usr/bin/env bash
#
# Runs the boundary benchmarks under node.
#
# Arguments are passed through to go test, so a single group can be run with
# ./run.sh -bench BenchmarkString

set -euo pipefail

cd "$(dirname "$0")"

go generate ./...

GOOS=js GOARCH=wasm go test \
	-exec="$(pwd)/node_exec.sh" \
	-run '^$' \
	-bench . \
	"$@" .
