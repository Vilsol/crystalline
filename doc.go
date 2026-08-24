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
//		r.Type(vendor.Client{}, bind.Without("internalHelper"))
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
// The generated module starts itself, and hands back the namespaces it bound:
//
//	import { boot } from "./dist/crystalline.js";
//
//	const { api } = await boot("main.wasm");
//
// boot takes a URL or the bytes of the binary, so a bundler that inlines the
// wasm and a test that reads it from disk both work. It needs the Go runtime
// shim, wasm_exec.js, to have been loaded already: that file is a plain script
// rather than a module, and it belongs to the toolchain that built the binary.
//
// The steps are available separately for a project that needs to place them
// itself. Each namespace is also a live export, and initializeCrystalline binds
// them once the wasm module is running. Reading one before that happens throws
// an explanation rather than a TypeError, and a namespace captured before
// initialisation says that it is a stale copy.
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
// Reaching the value needs no narrowing — unwrap and unwrapOr are there
// whichever half you hold — and checking ok narrows properly for code that
// would rather branch:
//
//	const config = api.Load(id).unwrap();
//
//	const result = api.Load(id);
//	if (result.ok) { use(result.value); } else { report(result.error); }
//
// A call that can fail is not necessarily slow, so it stays synchronous and
// does not force its callers to become async.
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
// or from the manifest with bind.AsPromise on the type, which checks the
// method exists. Calls
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
//	r.Type(api.Result{}, bind.Plain())
//
// Plain data has no methods, cannot be written back, and is an ordinary
// JavaScript object: it survives JSON, structuredClone and a framework's
// equality checks. The mark applies wherever the type appears, and to every
// struct reachable from it, since a plain value cannot contain a live one.
// The generated declarations show which types those are.
//
// # Regenerating on save
//
//	crystalline -watch -app myapp -out ./dist ./...
//
// Go files under -dir are polled, and a change regenerates. A generate that
// fails reports where and keeps watching, because a half-finished edit should
// leave the watcher waiting for the next save rather than exiting.
//
// # Measuring
//
// Generating with -profile counts and times every call made through the module,
// reported by stats():
//
//	greeting.Greet: 98 calls, 0.54ms
//
// An asynchronous call is timed until it settles rather than until it is handed
// back, so what is reported is the wait rather than the dispatch.
//
// It is off by default, because a counter and a clock reading on a five
// microsecond call are not free, and it counts calls on the module's own
// namespaces. A field read or a method on a struct wrapper is bound on the Go
// side and does not pass through it.
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
//	r.Type(api.Colour{}, bind.MarshalledBy(api.ColourToHex, api.ColourFromHex))
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
// # Calling out to JavaScript
//
// Everything else here goes one way: Go exports, JavaScript calls in. An
// interface parameter goes the other way. Go declares what it needs, and the
// app supplies an object with those methods:
//
//	type Recorder interface {
//		Record(event string)
//		Level() int
//	}
//
//	func Replay(r Recorder, events []string) int
//
//	// await api.Replay({ Record: (e) => log(e), Level: () => 7 }, events)
//
// The object is checked for the methods when it arrives, so a missing one is
// reported rather than found later. Each method is what a callback parameter
// already is: arguments convert out, the result is awaited in case JavaScript
// returned a promise, and it converts back. That makes the whole call a
// promise, as a callback does.
//
// A method returning more than one value is refused, since a JavaScript
// function returns one. This is deliberately the narrow form: it is not a way
// to bind arbitrary browser APIs from Go, which is a second generator's worth
// of work and the wrong shape for the cost of a crossing.
//
// # Reaching out to JavaScript
//
// An interface parameter lets JavaScript hand Go an object. An import fills a
// Go variable from an object JavaScript already has:
//
//	type Storage interface {
//		GetItem(key string) string
//		SetItem(key string, value string)
//	}
//
//	var Local Storage
//
//	// in the manifest
//	r.Import(&Local, bind.At("localStorage"))
//
// The interface is the whole contract. Nothing is generated from a description
// of the JavaScript API, so nothing is generated that nobody asked for, and the
// unions and overloads a real API description is full of never arise. Declare
// the four methods you call, not the ninety that exist.
//
// Every method is checked to exist while the bindings initialise. Nothing is
// calling then, so nothing can be told: the failures are published instead, and
// boot refuses to hand over a surface that is missing part of itself.
//
//	crystalline: Go could not reach what it imported:
//	api.Local: api.Storage: the object has no SetItem method
//
// A method is looked up under the name the rest of the surface uses, which is
// the Go name unless the whole surface was renamed. A browser API is spelled
// the JavaScript way, so name the ones that differ:
//
//	r.Import(&Saved, bind.At("localStorage"),
//		bind.Called("GetItem", "getItem"),
//		bind.Called("SetItem", "setItem"))
//
// A method is called synchronously unless it is declared otherwise. Awaiting
// one that was not declared would block the goroutine, and a goroutine blocked
// inside a synchronous call hands JavaScript undefined and finishes the work
// afterwards — a wrong answer rather than a slow one. So a promise arriving
// where none was declared is refused by name:
//
//	r.Import(&Client, bind.At("client"), bind.AsPromise("Fetch"))
//
// A declared one is awaited, which is safe only under something that is itself
// a promise: an exposed function marked bind.AsPromise, or one that is async
// already because it takes a callback, a context or a channel.
//
// This is deliberately not a way to drive the DOM from Go. A crossing costs
// about five microseconds whoever wrote the binding, so a thousand nodes is
// five milliseconds of boundary before any work happens. Compute in Go, render
// in JavaScript, and import the handful of things Go genuinely has to reach.
//
// # Enums
//
// Go spells an enum as a named integer or string type, a block of constants of
// that type, and usually a String method. Bound as the underlying type, all of
// that collapses into "number".
//
// The type is declared as the union of its values and its constants are bound
// beside it, so a caller names a value instead of writing the number the type
// happens to use:
//
//	type Phase int  ->  type Phase = 0 | 1 | 2;
//	                    const Phase: { readonly PhaseIdle: 0, ... };
//
// The constants keep their Go names. Trimming the type's name off the front is
// the usual convention, but it is a convention rather than a rule, and a name
// that matches on both sides is one a reader can grep for.
//
// # Embedding
//
// A promoted method is bound, so a call that compiles in Go works in
// JavaScript. An embedded value keeps its own name rather than being flattened
// into the outer object: reaching e.Base.Tag is one step further than Go's
// e.Tag, and it needs no rule about which field wins when names collide.
//
// # Naming
//
// An exported Go name is the JavaScript name, so one grep finds every use of it
// across both languages. That is the default because it is a live debugging aid
// in a two-language codebase, and it costs nothing.
//
// A surface whose consumers will never read the Go can ask for the other
// convention:
//
//	crystalline -case camel ...
//
// Timeout becomes timeout, ID becomes id, HTTPServer becomes httpServer. It
// applies to fields, methods and functions. Type names, namespaces, enum
// constants and the names given to r.Value are left alone: the first three are
// not members, and the last was written out by hand and is already whatever it
// was meant to be.
//
// A single member can be named outright instead, with or without the flag:
//
//	type Config struct {
//		TimeoutSeconds int `crystalline:"name=timeout"`
//	}
//
// The name has to be a JavaScript identifier, which is checked when generating.
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
// Where the range genuinely matters, tag the field and it crosses as a
// JavaScript BigInt, which carries it exactly:
//
//	type Ledger struct {
//		ID int64 `crystalline:"bigint"`
//	}
//
// The field is declared bigint rather than number, both directions are exact,
// a write that is not a BigInt is refused, and the wide-integer warning stops
// naming it because it is no longer a number. It is opt-in per field because it
// is not free on the other side: JSON.stringify throws on a bigint, and mixing
// one with a number is a TypeError, so the choice reaches the caller's
// arithmetic. The tag is refused on anything narrower than 64 bits.
//
// Reading one back cannot go through syscall/js at all. Value.Type panics with
// "bad type flag" on a BigInt, and Get and Call check the type first, so the
// conversion asks JavaScript for the digits and parses them.
//
// A variadic function takes its values as an array from JavaScript:
//
//	func Total(nums ...int) int   ->   Total(nums: Array<number>): number
//
// # TinyGo
//
// Every example builds and passes under TinyGo 0.41 with -scheduler=asyncify,
// at about a fifth of the size. The generated runtime needs js.FuncOf,
// channels, goroutines, sync.Once, sync.Mutex and generics, and no reflect,
// fmt or finalizers, which is the usual obstacle.
//
// TinyGo's wasm target implements no recover, so a panic aborts the module
// rather than arriving in JavaScript as an Error. Generated code does not panic
// to report a failure: a mistyped argument, an unknown property, a field that
// cannot be written, a value a channel could not carry and a source that
// rejects mid-stream are reported and returned instead. Two things still raise
// one — a panic in your own Go code, and a JavaScript callback or a method of a
// supplied object that returns the wrong type or rejects. The second converts
// inside a function whose Go signature is the consumer's, so there is nowhere
// to report to and nothing to return but a guess.
//
// # What it will not do
//
// Send-only channels, complex numbers and unsafe pointers have no JS
// counterpart. Anything that cannot be bound is reported by name and reason
// rather than quietly omitted, and a struct that can only be partly converted
// is declined outright, since dropping the rest would lose data silently.
package crystalline
