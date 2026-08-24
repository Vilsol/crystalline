# examples

Three pages, each one step further in. Every one is a real wasm binary built
from the Go beside it.

| | shows |
| --- | --- |
| [01-hello](01-hello) | one call each way: a string and an int |
| [02-accounts](02-accounts) | a struct with live fields and bound methods, `Result`, deterministic release |
| [03-pipeline](03-pipeline) | channels in both directions, a context as an `AbortSignal`, a promise by request |

## Running them

```sh
mise run examples        # generate, build and check every example under node
mise run examples:serve  # then open http://localhost:8000/01-hello/
```

## What is in each one

```
main.go             the manifest and the wasm entrypoint
<package>/          ordinary Go, with no knowledge of JavaScript
index.html          the page
smoke.mjs           the same calls, run under node

crystalline_gen.go  generated: the bindings
crystalline.js      generated: the ES module
crystalline.d.ts    generated: the declarations
```

The generated files are committed so they can be read here. To rebuild one by
hand, run `go generate ./...` in its directory.

This is a separate module, wired to the crystalline checkout above it with a
`replace` directive. A real project would depend on a released version instead:

```sh
go get -tool github.com/Vilsol/crystalline/cmd/crystalline
```
