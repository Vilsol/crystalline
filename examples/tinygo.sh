#!/usr/bin/env bash
#
# Spike: does the generated runtime work under TinyGo?
#
# 03-pipeline is the example to try, because it exercises the parts most likely
# to break: promises, channels in both directions, and cancellation through a
# context. It is the async surface that depends on a goroutine being able to
# block on a JS callback and resume, which is what -scheduler=asyncify provides.
#
# Reports the binary size either way, since size is the reason to want this.

set -euo pipefail

cd "$(dirname "$0")"

if ! command -v tinygo >/dev/null; then
	echo "tinygo is not installed: https://tinygo.org/getting-started/install/" >&2
	exit 1
fi

example="${1:-03-pipeline}"

echo "==> generating"
(cd "$example" && go generate ./...)

# TinyGo ships its own wasm_exec.js, and it does not match the one in GOROOT.
echo "==> wasm_exec.js from tinygo"
install -m 644 "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" "$example/wasm_exec.js"

echo "==> building with tinygo"
tinygo build -target wasm -scheduler=asyncify -o "$example/app.wasm" "./$example"

ls -lh "$example/app.wasm" | awk '{print "tinygo build: " $5}'

echo "==> running the smoke test"
node "$example/smoke.mjs"

echo
echo "For comparison, the same example under the standard toolchain:"
GOOS=js GOARCH=wasm go build -o /tmp/crystalline-go.wasm "./$example"
ls -lh /tmp/crystalline-go.wasm | awk '{print "go build:     " $5}'
