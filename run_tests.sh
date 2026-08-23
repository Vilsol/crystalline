#!/usr/bin/env bash

set -ex

rm -rf test.bin
GOOS=js GOARCH=wasm go test -c -o test.bin -covermode=atomic -coverpkg=./... ./
NODE_BIN=$(which node)
GOROOT=$(go env GOROOT)

# wasm_exec_node.js moved from misc/wasm to lib/wasm in Go 1.24
WASM_EXEC="$GOROOT/lib/wasm/wasm_exec_node.js"
if [ ! -f "$WASM_EXEC" ]; then
  WASM_EXEC="$GOROOT/misc/wasm/wasm_exec_node.js"
fi

env --ignore-environment $NODE_BIN "$WASM_EXEC" test.bin -test.v -test.coverprofile coverage.txt
rm -rf test.bin
