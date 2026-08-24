#!/usr/bin/env bash
#
# Typechecks the generated declarations.
#
# The declarations are text as far as the Go tests are concerned, so nothing
# else notices when they stop being valid TypeScript. That is how a wrapper
# came to be documented with a release() it never declared.
#
# Needs network the first time, to fetch the compiler.

set -euo pipefail

cd "$(dirname "$0")"

npx --yes typescript@5 tsc --project tsconfig.json
