# TODO

Work that is decided but not built, and work that was considered and declined.
Declined items carry their reasoning so they are not re-argued from scratch.

## Planned

### Naming an imported method

An import looks up each method under the name its own surface uses, so under the
default naming it asks JavaScript for `GetItem` and under `-case camel` it asks
for `getItem`. A real browser API is spelled the JavaScript way, which means
reaching one today requires `-case camel` for the whole surface. 05-ledger runs
into this and says so.

It fails loudly rather than silently — the boot names the method that is missing
— but the fix is a per-import spelling, the way `crystalline:"name=x"` works for
a field. A struct tag has nowhere to live on an interface method, so it would be
an option:

	r.Import(&Saved, bind.At("localStorage"), bind.Called("GetItem", "getItem"))

Worth doing when someone wants an import without wanting camelCase everywhere.

## Declined

### Generated batching

`bind.Batched()` emitting a `GetAll(keys)` beside `Get(key)`. A crossing costs
the same whoever writes the binding, so 98 small calls cost about half a
millisecond against 5.5 microseconds for one aggregate call, and that gap is
real.

It is declined because it is a trap unless it forces plain data: 98 wrappers at
about 91 microseconds each is slower than the per-call plain path it would be
sold as replacing. If it is ever built it must require `r.Plain` on the return
type and refuse at generate time otherwise, fail the whole batch rather than
carrying a `Result` per element, and let the varying parameter be declared
rather than assumed to be the first.

The evidence against it is that the second hot spot in a real migration was a
map read rather than a call, which batching cannot touch, and `r.Plain` fixed
it for nothing.

### A described browser API

`r.Import` shipped in 0.1.0: a Go variable of interface type, filled from an
object JavaScript already has. What did not ship, and should not, is a
description of the browser — the `web-sys` or `gowebapi` shape, where an API
definition is turned into bindings for everything.

Against it, from the ecosystems that tried:

- `web-sys` generates from WebIDL and then needs a Cargo feature **per type** to
  keep compile times and binary size usable. Generating only what a manifest
  declares means that problem cannot arise.
- `gowebapi` is the same idea for Go, over `syscall/js`. It is still
  experimental years on, missing namespace and union types, because WebIDL leans
  on unions and overloads and Go has neither. Its author put it plainly: hard to
  autogenerate, since Go is strictly typed without union types.
- The cost model does not want it. A crossing is about 5.5 microseconds, so
  driving the DOM per node loses to letting JavaScript render what Go computed.

Also left undone, and cheap when someone wants it: `bind.FromModule("./x.js")`,
the way Blazor scopes `[JSImport]` to a module rather than the global object.
The generated module would have to hand the namespace object to Go before init
runs, which is real plumbing rather than a one-liner.

### bigint and camelCase as defaults

Both shipped in 0.1.0 as opt-ins: `crystalline:"bigint"` on a field, and
`-case camel` or `crystalline:"name=..."` for naming.

Neither becomes the default. A bigint leaks into the caller's arithmetic —
`JSON.stringify` throws on one and mixing it with a number is a `TypeError` — so
every consumer would pay for a range most of them do not use. And an exported Go
name being the JavaScript name means one grep finds both sides of a
two-language codebase, which is worth more than matching a convention.

### Chasing full TinyGo support

It already works. Every example and the generator's own test surface — 82 checks
over every kind of crossing — produce identical results under TinyGo 0.41 and
under the standard toolchain, at roughly a fifth of the size. `./examples/tinygo.sh`
runs it. README says so, with the caveat.

Trialled on timeless-jewels, which is the consumer that cares most about the
cost of a crossing: it worked, and it was slower overall. `-scheduler=asyncify`
rewrites every function that can block, and that is not free. For a tool built
to squeeze the boundary, a fifth of the download does not pay for it.

So the remaining work is parked rather than planned:

- Two panics that cannot be reported: a JavaScript callback, or a method of a
  supplied object, that returns the wrong type or rejects. Both convert inside a
  Go function whose signature belongs to the consumer. They are the only two
  probe checks that cannot run under TinyGo, and closing them needs either a
  TinyGo with recover or a JS contract that cannot supply the wrong type.
- Whether TinyGo's conservative collector disturbs the handle table.

Revisit if a consumer turns up whose binary size matters more than its latency —
a page loaded once and thrown away rather than one used for an hour. Nothing has
to change for them today; they get the caveat and the size.

### Binary size attribution

Reporting which binding dragged which bytes into the wasm binary. Attribution
through the Go linker is approximate enough that people stop trusting the
numbers, which is worse than not reporting them.
