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

# The binary is tsc and the package is typescript, so the package has to be
# named separately: npx otherwise looks for an executable called typescript and
# reports only that it could not determine one.
npx --yes --package typescript@5 -- tsc --project tsconfig.json
