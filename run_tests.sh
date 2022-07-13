#!/usr/bin/env bash

set -ex

rm -rf test.bin
GOOS=js GOARCH=wasm go test -c -o test.bin -coverprofile -covermode=atomic -coverpkg=./... ./...
node "$(go env GOROOT)/misc/wasm/wasm_exec_node.js" test.bin -test.v -test.coverprofile coverage.txt
rm -rf test.bin
