// Package crystalline exposes Go code to JavaScript from a WebAssembly binary,
// and generates the TypeScript declarations to go with it.
//
// A Go program built with GOOS=js GOARCH=wasm can already reach JavaScript
// through syscall/js, but every binding is hand-written, lands in a flat
// global, and carries no type information. Crystalline reads your source and
// emits the bindings, an ES module and a .d.ts.
//
// Nothing it produces uses reflect. Reflection would have to run inside the
// wasm binary, which means shipping type metadata for everything reachable and
// discovering mistakes when a user first calls in rather than when you build.
// Reading the source instead keeps the binary smaller and moves every failure
// to generate time.
//
// # Declaring the surface
//
// A manifest is an ordinary function, marked so the generator can find it:
//
//	//crystalline:exports
//	func Exports(r bind.Registry) {
//		r.Func(api.Greet)
//		r.Func(api.Load, bind.AsPromise())
//		r.Ignore(vendor.Client{}, "internalHelper")
//		r.Value("Version", api.Version)
//	}
//
// Every symbol is checked by the compiler, so an upstream rename breaks the
// build rather than silently dropping a binding. Because the manifest names
// symbols rather than annotating them, it can expose packages you do not own.
//
// The generator reads the function to learn the types involved; the function
// itself runs at start-up to supply the values. Ordinary Go around the calls —
// locals, loops, conversions — needs no support here, since only the static
// type of each argument is required.
//
// For code you do own, //crystalline:export on a declaration is shorthand for a
// manifest entry.
//
// # Generating
//
//	//go:generate crystalline -app myapp -out ./dist ./...
//
// The command writes crystalline.js and crystalline.d.ts to -out, and the Go
// bindings next to the manifest that declared them. The same work is available
// as a library through Generator, for callers that want to place the output
// themselves.
//
// # Using the bindings
//
// The generated module exports one initializer, which must run after the wasm
// module has started:
//
//	import { initializeCrystalline, api } from "./dist/crystalline.js";
//
//	const go = new Go();
//	const { instance } = await WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject);
//	go.run(instance);
//	initializeCrystalline();
//
// Calling it too early throws with an explanation rather than a TypeError.
//
// # How Go concepts arrive
//
// Failure, streams and cancellation exist in both languages, so each is mapped
// to the counterpart that means the same thing rather than to Go's spelling of
// it.
//
// A trailing error becomes a Result:
//
//	func Load(id string) (Config, error)   ->   Load(id: string): Result<Config>
//
// Result is one interface with unwrap and unwrapOr, not a union to narrow. A
// call that can fail is not necessarily slow, so it stays synchronous and does
// not force its callers to become async.
//
// A receive-only channel becomes an AsyncIterable, consumable with for await. A
// leading context.Context becomes an AbortSignal, which is the only way to
// interrupt Go work that would otherwise hold the single JS thread.
//
// A panic is different from an error: it arrives as a thrown or rejected Error
// carrying the stack, because it is a bug rather than an expected outcome.
//
// # Promises
//
// A Go call blocks the single JS thread. Marking a function runs it on its own
// goroutine and hands JavaScript a Promise instead:
//
//	r.Func(api.Load, bind.AsPromise())
//
// Methods can be marked with a comment on the declaration, which also flows
// into the generated types:
//
//	// crystalline:promise
//	func (s Service) Load() Result { ... }
//
// or from the manifest with r.Promise, which checks the method exists. Calls
// taking a callback or a context are always promises, whether or not they are
// marked: neither can be serviced without yielding to the event loop.
//
// # Structs
//
// A struct arrives as an object whose fields read and write through to the Go
// value, with its methods bound alongside. Handing one back to Go resolves it
// to the value it came from rather than a copy, and a plain object literal is
// accepted in its place.
//
// Wrappers hold resources in the Go/JS bridge. They are released when
// JavaScript collects them, or deterministically:
//
//	using config = api.LoadConfig();
//
// # Plain data
//
// A wrapper is a live view, and liveness is not free: every field read is a
// call into Go, and every field and method holds a slot in the Go/JS bridge
// until the wrapper is released. A result that JavaScript only reads pays for
// machinery it never uses.
//
// Marking the type in the manifest converts it once instead:
//
//	r.Plain(api.Result{})
//
// Plain data has no methods, cannot be written back, and is an ordinary
// JavaScript object: it survives JSON, structuredClone and a framework's
// equality checks. The mark applies wherever the type appears, and to every
// struct reachable from it, since a plain value cannot contain a live one.
// The generated declarations show which types those are.
//
// # Cost
//
// Crossing the boundary costs about the same whoever writes the binding, so
// the thing to reduce is the number of crossings rather than the work in each.
// One call returning an aggregate beats many small ones, a field read in a loop
// is a call in a loop, and bulk data travels faster as []byte than as a string.
// The bench directory measures each kind of crossing.
//
// # The generated file
//
// crystalline_gen.go is written next to the manifest and carries a js build
// tag. Commit it, along with the JavaScript and the declarations: a plain go
// build for wasm then works without running the generator first, and a change
// to the JavaScript surface shows up in review rather than at deploy time.
//
// Output is deterministic, so CI can check that what is committed is what the
// generator currently produces:
//
//	go generate ./...
//	git add -N path/to/generated
//	git diff --exit-code path/to/generated
//
// The add -N matters. A diff on its own ignores files nobody has committed yet,
// so the first generated file in a project passes the check in silence, which
// is exactly when you least want it to.
//
// # Mapping a type onto a counterpart
//
// A struct is bound as a live view of its exported fields, which says nothing
// useful about a type whose value is not its fields. A pair of ordinary Go
// functions maps one onto something JavaScript already has:
//
//	r.Marshal(api.ColourToHex, api.ColourFromHex)
//
// The signatures carry the declaration. func(Colour) string says Colour crosses
// as a string; func(string) (Colour, error) says how it comes back and that it
// may refuse. Both are checked when generating, so a mapping that does not line
// up fails at the manifest rather than several steps away in emitted code.
//
// time.Time and time.Duration are mapped this way already, to a Date and to a
// number of milliseconds. Before that, time.Time bound as a wrapper carrying
// thirty methods and no readable data, and a real Date passed to it was
// silently read as the zero time.
//
// # Embedding
//
// A promoted method is bound, so a call that compiles in Go works in
// JavaScript. An embedded value keeps its own name rather than being flattened
// into the outer object: reaching e.Base.Tag is one step further than Go's
// e.Tag, and it needs no rule about which field wins when names collide.
//
// # Struct tags
//
// A nil slice or map maps to null, which JS code expecting a collection
// usually does not want. The not_nil option emits an empty collection instead,
// and makes the field non-optional in the generated types:
//
//	type Config struct {
//		Hosts []string `crystalline:"not_nil"`
//	}
//
// Unrecognised options are rejected rather than ignored.
//
// # Wide integers
//
// A JavaScript number is a double, so an int64 or uint64 beyond 2^53 is rounded
// rather than carried. Refusing those types would break ordinary Go, where
// identifiers and timestamps are routinely int64, so they are bound as numbers
// and every member that traffics in one is named at generate time:
//
//	crystalline: warning api.Timestamps: int64 is bound as a JavaScript number,
//	which cannot represent values beyond 2^53 exactly
//
// If the range matters, carry the value as a string across the boundary.
//
// A variadic function takes its values as an array from JavaScript:
//
//	func Total(nums ...int) int   ->   Total(nums: Array<number>): number
//
// # What it will not do
//
// Send-only channels, complex numbers and unsafe pointers have no JS
// counterpart. Anything that cannot be bound is reported by name and reason
// rather than quietly omitted, and a struct that can only be partly converted
// is declined outright, since dropping the rest would lose data silently.
package crystalline
