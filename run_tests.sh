#!/usr/bin/env bash

set -ex

# Crystalline is a build-time tool, so the suite runs natively. The generated
# bindings are exercised by building and running a wasm binary from within the
# tests, which is where js/wasm coverage comes from.
go test -covermode=atomic -coverprofile=coverage.txt ./...
