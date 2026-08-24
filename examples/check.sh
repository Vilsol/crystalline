#!/usr/bin/env bash
#
# Builds every example and runs it under node, so an example that has stopped
# working fails here rather than in someone's browser.

set -euo pipefail

cd "$(dirname "$0")"

./build.sh "$@"

for smoke in */smoke.mjs; do
	node "$smoke"
done
