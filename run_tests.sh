#!/usr/bin/env bash

set -ex

rm -rf test.bin
GOOS=js GOARCH=wasm go test -c -o test.bin -coverprofile -covermode=atomic -coverpkg=./... ./
NODE_BIN=$(which node)
env --ignore-environment $NODE_BIN "$(go env GOROOT)/lib/wasm/wasm_exec_node.js" test.bin -test.v -test.coverprofile coverage.txt
rm -rf test.bin
