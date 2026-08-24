#!/usr/bin/env bash
#
# Generates bindings and builds a wasm binary for each example.
#
# Pass example directory names to build a subset; with no arguments it builds
# every example.

set -euo pipefail

cd "$(dirname "$0")"

examples=("$@")
if [ ${#examples[@]} -eq 0 ]; then
	examples=(*/)
fi

wasm_exec="$(go env GOROOT)/lib/wasm/wasm_exec.js"
if [ ! -f "$wasm_exec" ]; then
	wasm_exec="$(go env GOROOT)/misc/wasm/wasm_exec.js"
fi

for example in "${examples[@]}"; do
	example="${example%/}"

	echo "==> $example"

	(cd "$example" && go generate ./...)

	# install rather than cp: the file in GOROOT is read-only.
	install -m 644 "$wasm_exec" "$example/wasm_exec.js"

	GOOS=js GOARCH=wasm go build -o "$example/app.wasm" "./$example"
done
