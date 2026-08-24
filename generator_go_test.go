//go:build !js

package crystalline

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MarvinJWendt/testza"
)

// TestGeneratedBindingsAreReflectFree is the point of the static path: bindings
// emitted from source register through syscall/js directly, so the linker never
// sees reflect and can drop the type metadata it would otherwise pin.
//
// It builds a throwaway module and runs it under node, so it needs a toolchain
// and is skipped in short mode.
func TestGeneratedBindingsAreReflectFree(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a wasm binary and runs it under node")
	}

	dir := buildGeneratedModule(t)

	deps := goCommand(t, dir, "list", "-deps", ".")
	for _, dep := range strings.Split(deps, "\n") {
		testza.AssertNotEqual(t, "reflect", strings.TrimSpace(dep),
			"generated bindings must not pull reflect into the binary")
	}
}

// TestGeneratedBindingsWork exercises the whole emitted surface end to end:
// values, fields, methods, results, streams, cancellation and disposal.
func TestGeneratedBindingsWork(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a wasm binary and runs it under node")
	}

	dir := buildGeneratedModule(t)

	reported := probeResults(t, runWasm(t, dir))

	for _, expected := range []string{
		"Basic=420",
		"FirstValue=hello",
		"SecondValue=123",
		"A(true)=true",
		// C carries // crystalline:promise, so it must hand back a Promise.
		"C_isPromise=true",
		"One()=hello",
		// not_nil must produce an empty array, matching what the .d.ts declares.
		"NeverNil=[]",
		// An untagged nil map stays null.
		"Lookup=null",
		// An error-only method reports success without becoming async.
		"Fails=true",
		// The callback is invoked from Go and its result awaited.
		"Callback=true",
		"CallbackBad=threw",
		"TextBad=threw",
		"TextGood=x!",
		"NullStruct=threw",
		"WrongBytes=threw",
		"BadPromiseArg=rejected",
		"AfterBadPromise=420",
		// Assigning a field from JS must reach the Go value behind it.
		"WriteThrough=written",
		// A field write that cannot be converted used to be discarded in
		// silence, leaving the Go value untouched and reporting nothing.
		"SliceWrite=[\"written\"]",
		"MapWrite={\"a\":1}",
		"BytesWrite=7",
		"StructWrite=nested",
		"ReadOnlyWrite=threw",
		"NestedIdentity=true",
		"PointerIdentity=true",
		"Pointed=pointed",
		"NestedLive=via-cache",
		"NestedReaches=via-cache",
		"SliceIdentity=false",
		"PlainLength=2",
		"PlainLabel=reading",
		"PlainValues=[1,2]",
		"PlainNoRelease=true",
		"PlainNested=noon",
		"PlainNestedNoMethod=true",
		"PlainJSON={\"At\":\"noon\",\"Value\":1}",
		"BigType=bigint",
		"BigRead=9007199254740993",
		"BigUnsigned=18446744073709551615",
		"BigRounded=9007199254740992",
		"BigWrite=9007199254740995",
		"BigBad=threw",
		"BigRange=threw",
		"Renamed=exact",
		"RenamedGone=true",
		"RenamedWrites=written",
		"StructIdentity=written",
		"StructLiteral=literal",
		"TypoRejected=yes",
		"Apply=3",
		"ApplyIdentity=0",
		// Fallible calls stay synchronous and carry a Result.
		"MayFailOk=fine",
		"MayFailErr=asked to fail",
		"MayFailOr=fallback",
		"OnlyFailsOk=true",
		"SharedProto=true",
		"OwnKeys=ok,value",
		// Streams, in both directions.
		"Stream=item,item,item",
		"Total=15",
		"Promoted=tagged",
		"Direct=own",
		"EmbeddedField=tagged",
		"TimeIn=2020-01-02T03:04:05Z",
		"TimeOut=true",
		"TimeValue=2020-01-02T03:04:05.000Z",
		"BadTime=threw",
		"MaybeStamp=2020-01-02T03:04:05.000Z",
		"MaybeStampNil=null",
		"RateKey=1500",
		"Colour=#203040",
		"BadColour=threw",
		"EnumIdle=0",
		"EnumDone=2",
		"EnumAdvance=1",
		"SuppliedLevel=7",
		"SuppliedSeen=a,b",
		"SuppliedMissing=threw",
		"PairString=a",
		"PairSwapped=b",
		"PairNumber=2",
		"SumArray=6",
		"SumAsync=30",
		"BadFeed=threw",
		"RejectingFeed=threw",
		"First=1",
		"FeedStopped=true",
		// A pointer parameter takes null as well as a value.
		"MiddleNull=!",
		"MiddleValue=hello!",
		// A returned stream outlives the call that produced it.
		"TicksNotPromise=true",
		"Ticks=0,1,2,3",
		"TicksAborted=ended",
		"TicksBroke=ok",
		// Cancellation.
		"Cancellable=live",
		"Aborted=yes",
		// Explicit disposal.
		"Disposed=yes",
	} {
		key, want, _ := strings.Cut(expected, "=")

		got, ok := reported[key]
		testza.AssertTrue(t, ok, "the probe never reported "+key)
		testza.AssertEqual(t, want, got, key+" was wrong")
	}
}

// probeResults splits the probe's output into one value per assertion.
//
// Matching each expectation as a substring of the whole output was weaker than
// it looked: "Ticks=0,1,2,3" passed on five ticks, "First=1" on "First=10", and
// "PlainLength=2" on twenty. Exact comparison per key is what was meant.
func probeResults(t *testing.T, output string) map[string]string {
	t.Helper()

	results := make(map[string]string)

	for _, pair := range strings.Split(strings.TrimSpace(output), " | ") {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}

		results[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	testza.AssertTrue(t, len(results) > 0, "the probe reported nothing:\n"+output)

	return results
}

// TestGeneratedBindingsReportGaps pins that anything the emitter cannot bind is
// reported rather than silently dropped, and that its bindable siblings still
// get bound.
func TestGeneratedBindingsReportGaps(t *testing.T) {
	pkg, err := generateBindings(t, "./testdata/nobind")
	testza.AssertNoError(t, err)

	reported := make([]string, 0, len(pkg.Skipped))
	for _, skipped := range pkg.Skipped {
		reported = append(reported, skipped.String())
	}

	joined := strings.Join(reported, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "Send"),
		"a send-only channel must be reported, got:\n"+joined)
	testza.AssertFalse(t, strings.Contains(joined, "Fine"),
		"a bindable sibling must still bind, got:\n"+joined)
	testza.AssertTrue(t, strings.Contains(pkg.Source, "crystallineFnUnbindableFine"),
		"Fine must be bound:\n"+pkg.Source)
}

// buildGeneratedModule writes the generated bindings into a throwaway module
// and builds it for wasm, returning the directory.
//
// The module resolves crystalline through a replace directive, which is how a
// consumer would wire it up.
func buildGeneratedModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	repo, err := filepath.Abs(".")
	testza.AssertNoError(t, err)

	pkg, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)
	testza.AssertNoError(t, os.WriteFile(filepath.Join(dir, "crystalline_gen.go"), []byte(pkg.Source), 0o644))

	gomod := "module bindtest\n\ngo 1.27\n\nrequire github.com/Vilsol/crystalline v0.0.0\n\nreplace github.com/Vilsol/crystalline => " + repo + "\n"
	testza.AssertNoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))
	testza.AssertNoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(probeMain), 0o644))

	sum, err := os.ReadFile("go.sum")
	testza.AssertNoError(t, err)
	testza.AssertNoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644))

	goCommand(t, dir, "mod", "tidy")
	goCommand(t, dir, "build", "-ldflags=-s -w", "-o", "app.wasm", ".")

	return dir
}

// generateBindings runs the generator over a manifest package and returns the
// bindings for a module named bindtest.
func generateBindings(t *testing.T, manifest string) (GoBindings, error) {
	t.Helper()

	g := NewGenerator("app")
	if err := g.Load(".", manifest+"/..."); err != nil {
		return GoBindings{}, err
	}

	declarations, err := g.Declarations()
	if err != nil {
		return GoBindings{}, err
	}

	return g.BuildGo(declarations, WithPackageName("main"), WithImportPath("bindtest"))
}

func goCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")

	out, err := cmd.CombinedOutput()
	testza.AssertNoError(t, err, string(out))

	return string(out)
}

func runWasm(t *testing.T, dir string) string {
	t.Helper()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}

	root := strings.TrimSpace(runGo(t, "env", "GOROOT"))

	exec_ := filepath.Join(root, "lib", "wasm", "wasm_exec_node.js")
	if _, err := os.Stat(exec_); err != nil {
		exec_ = filepath.Join(root, "misc", "wasm", "wasm_exec_node.js")
	}

	cmd := exec.Command(node, exec_, filepath.Join(dir, "app.wasm"))

	// The Go wasm runtime caps the combined size of argv and the environment,
	// so hand it a minimal one rather than the whole inherited environment.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

	out, err := cmd.CombinedOutput()
	testza.AssertNoError(t, err, string(out))

	return string(out)
}

func runGo(t *testing.T, args ...string) string {
	t.Helper()

	out, err := exec.Command("go", args...).Output()
	testza.AssertNoError(t, err)

	return string(out)
}

const probeMain = `package main

import "syscall/js"

func main() {
	result := js.Global().Call("eval", ` + "`" + `(async () => {
		const s = globalThis.go.app.sample;
		const g = globalThis.go.app.generic;
		const out = [];
		out.push("Basic=" + s.Basic());
		const f = s.FooBar();
		out.push("FirstValue=" + f.FirstValue);
		out.push("SecondValue=" + f.SecondValue);
		out.push("A(true)=" + f.A(true));
		out.push("C_isPromise=" + (typeof f.C(true).then === "function"));
		out.push("One()=" + f.One());
		const r = s.Rich();
		out.push("NeverNil=" + JSON.stringify(r.NeverNil));
		out.push("Lookup=" + JSON.stringify(r.Lookup));
		out.push("Fails=" + r.Fails().ok);
		out.push("Callback=" + await r.WithCallback((v) => v.length));

		// A callback that returns nothing must not be read as a value. Go used
		// to take js.Value.String() of undefined, which is the literal
		// "<undefined>", and Float() of it, which is NaN.
		let callbackBad = "accepted";
		try {
			await r.WithCallback((v) => {});
		} catch (e) {
			callbackBad = e.message.includes("expected a number") ? "threw" : "wrong:" + e.message;
		}
		out.push("CallbackBad=" + callbackBad);

		// The string case is the one that was silent: Go received the literal
		// "<undefined>" and carried on.
		let textBad = "accepted";
		try {
			await r.WithText((v) => {});
		} catch (e) {
			textBad = e.message.includes("expected a string") ? "threw" : "wrong:" + e.message;
		}
		out.push("TextBad=" + textBad);
		out.push("TextGood=" + await r.WithText((v) => v + "!"));

		// A promise-returning call whose argument cannot be converted must
		// reject. The failure travels through the same slot a synchronous call
		// uses, and the promise has to read it: resolving with whatever the
		// body returned would hand JavaScript undefined and call it success.
		let badPromiseArg = "resolved";
		try {
			await s.Cancellable(undefined, 5);
		} catch (e) {
			badPromiseArg = e.message.includes("expected a string") ? "rejected" : "wrong:" + e.message;
		}
		out.push("BadPromiseArg=" + badPromiseArg);

		// The slot must be clear afterwards, or the next synchronous call
		// throws someone else's error.
		out.push("AfterBadPromise=" + s.Basic());

		// A struct parameter is not optional: null must not arrive in Go as a
		// zero value nobody asked for.
		let nullStruct = "accepted";
		try {
			r.Configure(null);
		} catch (e) {
			nullStruct = e.message.includes("expected an object, got null") ? "threw" : "wrong:" + e.message;
		}
		out.push("NullStruct=" + nullStruct);

		// Every typed array has a byteLength, so checking for one accepted any
		// of them and then copied nothing.
		let wrongBytes = "accepted";
		try {
			r.Apply({Blob: new Int32Array([1, 2, 3])});
		} catch (e) {
			wrongBytes = e.message.includes("expected a Uint8Array") ? "threw" : "wrong:" + e.message;
		}
		out.push("WrongBytes=" + wrongBytes);
		const live = s.FooBar();
		live.FirstValue = "written";
		out.push("WriteThrough=" + live.One());

		// Every writable field must reach the Go value, not just the scalars.
		r.NeverNil = ["written"];
		out.push("SliceWrite=" + JSON.stringify(r.NeverNil));
		r.Lookup = {a: 1};
		out.push("MapWrite=" + JSON.stringify(r.Lookup));
		r.Blob = new Uint8Array([7]);
		out.push("BytesWrite=" + (r.Blob ? r.Blob[0] : "null"));
		r.Inner = {FirstValue: "nested"};
		out.push("StructWrite=" + r.Inner.FirstValue);

		// A field that genuinely cannot be written must say so, not accept the
		// write and drop it.
		const ticker = s.NewTicker();
		try {
			ticker.Events = [];
			out.push("ReadOnlyWrite=accepted");
		} catch (e) {
			out.push("ReadOnlyWrite=" + (e.message.includes("cannot be written") ? "threw" : e.message));
		}

		// Reading a struct field twice must give the same object: a fresh
		// wrapper per read breaks ===, Map keys and every memo comparison, and
		// allocates a handle each time.
		out.push("NestedIdentity=" + (r.Inner === r.Inner));
		out.push("PointerIdentity=" + (r.Pointed === r.Pointed));
		out.push("Pointed=" + r.Pointed.FirstValue);

		// Liveness must survive that: the cached wrapper still reaches Go.
		r.Inner.FirstValue = "via-cache";
		out.push("NestedLive=" + r.Inner.FirstValue);
		out.push("NestedReaches=" + r.Configure(r.Inner));

		// A slice field is a snapshot rather than a view, so it must not be
		// cached: a Go-side change has to show up on the next read.
		out.push("SliceIdentity=" + (r.NeverNil === r.NeverNil));

		// Plain data: converted once, no handle to release, no methods, and
		// ordinary JavaScript objects all the way down.
		const readings = s.Readings(2);
		out.push("PlainLength=" + readings.length);
		out.push("PlainLabel=" + readings[0].Label);
		out.push("PlainValues=" + JSON.stringify(readings[0].Values));
		out.push("PlainNoRelease=" + (readings[0].release === undefined));
		out.push("PlainNested=" + readings[0].Peak.At);
		out.push("PlainNestedNoMethod=" + (readings[0].Peak.Describe === undefined));
		out.push("PlainJSON=" + JSON.stringify(readings[1].Peak));

		// A field tagged bigint carries its value exactly. syscall/js cannot
		// look at a BigInt at all — Value.Type panics with "bad type flag" and
		// Get checks the type first — so the conversion asks JavaScript for the
		// digits and parses them.
		const ledger = s.NewLedger();
		out.push("BigType=" + typeof ledger.ID);
		out.push("BigRead=" + ledger.ID);
		out.push("BigUnsigned=" + ledger.Balance);

		// The same number in an untagged field, which is the cost being opted
		// out of: a double cannot hold it and rounds down.
		out.push("BigRounded=" + ledger.Rounded);

		ledger.ID = 9007199254740995n;
		out.push("BigWrite=" + ledger.ID);

		let bigBad = "accepted";
		try {
			ledger.ID = 5;
		} catch (e) {
			bigBad = e.message.includes("expected a bigint") ? "threw" : "wrong:" + e.message;
		}
		out.push("BigBad=" + bigBad);

		// A field named outright, with camelCase off. The Go name must be gone
		// rather than there as well.
		out.push("Renamed=" + ledger.memo);
		out.push("RenamedGone=" + (ledger.Note === undefined));
		out.push("RenamedWrites=" + (() => { ledger.memo = "written"; return ledger.memo; })());

		let bigRange = "accepted";
		try {
			ledger.Balance = -1n;
		} catch (e) {
			bigRange = e.message.includes("does not fit in a uint64") ? "threw" : "wrong:" + e.message;
		}
		out.push("BigRange=" + bigRange);

		// A wrapper handed back to Go must resolve to the same Go value.
		out.push("StructIdentity=" + r.Configure(live));

		// A plain object literal must still be accepted.
		out.push("StructLiteral=" + r.Configure({FirstValue: "literal"}));

		// A misspelled property must be rejected, not silently zeroed.
		let rejected = "no";
		try {
			r.Configure({FirstVlaue: "typo"});
		} catch (e) {
			rejected = e.message.includes('unknown property "FirstVlaue"') ? "yes" : "wrong:" + e.message;
		}
		out.push("TypoRejected=" + rejected);

		// Every field kind must convert back from JS, or the call throws.
		out.push("Apply=" + r.Apply({
			Blob: new Uint8Array([1, 2, 3]),
			Lookup: {a: 1, b: 2},
			MightBeNil: ["a", "b", "c"],
			NeverNil: ["x"],
			Inner: {FirstValue: "nested"},
			Pointed: null
		}));

		// A wrapper passed back must resolve by identity here too.
		out.push("ApplyIdentity=" + r.Apply(r));

		// Fallible but synchronous: no await, no async caller, no narrowing.
		out.push("MayFailOk=" + s.MayFail(true).unwrap());
		try {
			s.MayFail(false).unwrap();
			out.push("MayFailErr=no throw");
		} catch (e) {
			out.push("MayFailErr=" + e.message);
		}
		out.push("MayFailOr=" + s.MayFail(false).unwrapOr("fallback"));
		out.push("OnlyFailsOk=" + s.OnlyFails(true).ok);

		// The methods come from a shared prototype, so they are not own
		// properties and cost nothing per call.
		const shared = Object.getPrototypeOf(s.MayFail(true)) === Object.getPrototypeOf(s.MayFail(false));
		out.push("SharedProto=" + shared);
		out.push("OwnKeys=" + Object.keys(s.MayFail(true)).join(","));

		const streamed = [];
		for await (const item of s.Stream(3)) {
			streamed.push(item);
		}
		out.push("Stream=" + streamed.join(","));

		// A pointer parameter accepts null and a wrapper alike.
		out.push("MiddleNull=" + s.Middle(null, "!"));
		out.push("MiddleValue=" + s.Middle(s.FooBar(), "!"));

		// A context governs a returned stream, not the call that hands it
		// back, so the stream must survive the call returning.
		const ticking = s.Ticks(undefined, 4);
		out.push("TicksNotPromise=" + (ticking.then === undefined));

		const ticks = [];
		for await (const tick of ticking) {
			ticks.push(tick);
		}
		out.push("Ticks=" + ticks.join(","));

		// Aborting must end a stream that would otherwise run for a long time.
		const stopper = new AbortController();
		let seen = 0;
		for await (const tick of s.Ticks(stopper.signal, 100000)) {
			seen++;
			if (seen === 2) { stopper.abort(); }
		}
		out.push("TicksAborted=" + (seen < 10 ? "ended" : "ran " + seen));

		// Breaking out of the loop must not hang.
		const broken = new AbortController();
		for await (const tick of s.Ticks(broken.signal, 100000)) {
			break;
		}
		out.push("TicksBroke=ok");

		out.push("Cancellable=" + (await s.Cancellable(undefined, "live")).unwrap());

		// A channel parameter accepts anything iterable.
		out.push("Total=" + s.Total([4, 5, 6]));

		// A method promoted from an embedded field is callable, as it is in Go.
		const embedder = s.MakeEmbedder();
		out.push("Promoted=" + embedder.Promoted());
		out.push("Direct=" + embedder.Direct());
		out.push("EmbeddedField=" + embedder.Base.Tag);

		// A time crosses as a Date, both ways. It used to arrive as a wrapper
		// with no readable fields, and a real Date was silently read as the
		// zero time because there were no known properties to reject.
		out.push("TimeIn=" + s.TakesTime(new Date(Date.UTC(2020, 0, 2, 3, 4, 5))));
		const stamped = s.MakeStamped();
		out.push("TimeOut=" + (stamped.At instanceof Date));
		out.push("TimeValue=" + stamped.At.toISOString());
		let badTime = "accepted";
		try {
			s.TakesTime({});
		} catch (e) {
			badTime = e.message.includes("expected a Date") ? "threw" : "wrong:" + e.message;
		}
		out.push("BadTime=" + badTime);

		// A pointer to a mapped type crosses as its counterpart, or as null.
		out.push("MaybeStamp=" + s.MaybeStamp(true).toISOString());
		out.push("MaybeStampNil=" + JSON.stringify(s.MaybeStamp(false)));

		// A mapped type used as a key crosses the same way it does as a value.
		out.push("RateKey=" + Object.keys(s.Rates())[0]);

		// A declared mapping: Colour crosses as a string, both ways.
		const m = globalThis.go.app.marshal;
		out.push("Colour=" + m.Brighten("#102030"));
		let badColour = "accepted";
		try {
			m.Brighten("nonsense");
		} catch (e) {
			badColour = e.message.includes("expected a colour like #aabbcc") ? "threw" : "wrong:" + e.message;
		}
		out.push("BadColour=" + badColour);

		// An enum's constants are nameable, and the type still crosses as its
		// underlying number.
		out.push("EnumIdle=" + s.Phase.PhaseIdle);
		out.push("EnumDone=" + s.Phase.PhaseDone);
		out.push("EnumAdvance=" + s.Advance(s.Phase.PhaseIdle));

		// The other direction: Go declares what it needs, JavaScript supplies
		// an object with those methods and Go calls out through it.
		const recorded = [];
		const level = await s.Replay({
			Record: (event) => { recorded.push(event); },
			Level: () => 7,
		}, ["a", "b"]);
		out.push("SuppliedLevel=" + level);
		out.push("SuppliedSeen=" + recorded.join(","));

		let missingMethod = "accepted";
		try {
			await s.Replay({ Record: () => {} }, []);
		} catch (e) {
			missingMethod = e.message.includes("the object has no Level method") ? "threw" : "wrong:" + e.message;
		}
		out.push("SuppliedMissing=" + missingMethod);

		// Distinct instantiations of one generic type, actually called.
		out.push("PairString=" + g.Strings().First);
		out.push("PairSwapped=" + g.Strings().Swapped().First);
		out.push("PairNumber=" + g.Numbers().Second);
		out.push("SumArray=" + await s.Sum([1, 2, 3]));
		async function* generated() { yield 10; yield 20; }
		out.push("SumAsync=" + await s.Sum(generated()));

		// A value the channel cannot carry must fail the call, not end the
		// stream early. Go used to see a clean EOF after the good values and
		// return a plausible answer for a truncated input.
		let badFeed = "accepted";
		try {
			await s.Sum([1, "two", 3]);
		} catch (e) {
			badFeed = e.message.includes("values: expected a number") ? "threw" : "wrong:" + e.message;
		}
		out.push("BadFeed=" + badFeed);

		// A source that rejects mid-stream must fail the call. The feed awaited
		// each step through a helper that panics on rejection, inside a bare
		// goroutine with no recover, so one rejected next() took the whole
		// module down under either toolchain.
		let rejectingFeed = "accepted";
		try {
			async function* failing() { yield 1; throw new Error("source failed"); }
			await s.Sum(failing());
		} catch (e) {
			rejectingFeed = e.message.includes("source failed") ? "threw" : "wrong:" + e.message;
		}
		out.push("RejectingFeed=" + rejectingFeed);

		// Abandoning the channel must stop the feed rather than strand it.
		let stopped = false;
		const endless = {
			[Symbol.asyncIterator]() {
				let n = 0;
				return {
					next: async () => ({ value: ++n, done: false }),
					return: async () => { stopped = true; return { done: true }; },
				};
			},
		};
		out.push("First=" + await s.First(endless));
		out.push("FeedStopped=" + stopped);
		const controller = new AbortController();
		controller.abort();
		const aborted = await s.Cancellable(controller.signal, "dead");
		out.push("Aborted=" + (aborted.ok ? "no" : "yes"));

		// Explicit disposal frees the handle, so passing the wrapper back must
		// fail rather than silently resolving to a stale object.
		const disposable = s.FooBar();
		disposable.release();
		try {
			r.Configure(disposable);
			out.push("Disposed=no");
		} catch (e) {
			out.push("Disposed=" + (e.message.includes("released") ? "yes" : e.message));
		}
		return out.join(" | ");
	})()` + "`" + `)

	done := make(chan struct{})

	result.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
		js.Global().Get("console").Call("log", args[0].String())
		close(done)

		return nil
	}), js.FuncOf(func(this js.Value, args []js.Value) any {
		js.Global().Get("console").Call("log", "REJECTED: "+args[0].Call("toString").String())
		close(done)

		return nil
	}))

	<-done
}
`

// TestFallibleCallsStaySynchronous pins the point of the Result type: a call
// that can fail is usable without awaiting it, so it does not force its callers
// to become async.
func TestFallibleCallsStaySynchronous(t *testing.T) {
	pkg, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	// MayFail returns (string, error) and nothing else makes it async, so its
	// wrapper must not defer onto a goroutine.
	wrapper := pkg.Source[strings.Index(pkg.Source, "func crystallineFnSampleMayFail"):]
	wrapper = wrapper[:strings.Index(wrapper, "\n}\n")]

	testza.AssertFalse(t, strings.Contains(wrapper, "crystallinePromise"),
		"a fallible but synchronous call must not become a promise:\n"+wrapper)
	testza.AssertTrue(t, strings.Contains(wrapper, "crystallineErr(r1)"),
		"failure must travel in the value:\n"+wrapper)
	testza.AssertTrue(t, strings.Contains(wrapper, "crystallineOk("),
		"success must travel in the value:\n"+wrapper)
}

// TestUndirectedChannelParameterIsReported pins that the Go emitter refuses the
// same thing the declarations do, rather than binding something the .d.ts does
// not describe.
func TestUndirectedChannelParameterIsReported(t *testing.T) {
	pkg, err := generateBindings(t, "./testdata/bidiparam")
	testza.AssertNoError(t, err)

	reported := make([]string, 0, len(pkg.Skipped))
	for _, skipped := range pkg.Skipped {
		reported = append(reported, skipped.String())
	}

	joined := strings.Join(reported, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "must say its direction"),
		"an undirected channel parameter must be reported, got:\n"+joined)
}

// TestStreamedContextOutlivesTheCall pins the lifetime rule a returned channel
// needs: cancelling on the way out of the call ended the stream before
// JavaScript had read anything from it, which showed up as an empty iterable
// rather than as an error.
func TestStreamedContextOutlivesTheCall(t *testing.T) {
	pkg, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	streaming := wrapperBody(t, pkg.Source, "crystallineFnSampleTicks")

	testza.AssertFalse(t, strings.Contains(streaming, "defer crystallineStop()"),
		"a returned stream must not have its context cancelled when the call returns:\n"+streaming)
	testza.AssertTrue(t, strings.Contains(streaming, "}, crystallineStop)"),
		"the stream must take over the cancellation:\n"+streaming)

	// A stream that fails before it exists has nothing to take the context
	// over, so the call must tear it down itself.
	failing := wrapperBody(t, pkg.Source, "crystallineFnSampleStreamable")

	failed := failing[strings.Index(failing, "if r1 != nil"):]

	testza.AssertTrue(t, strings.Contains(failed[:strings.Index(failed, "}")], "crystallineStop()"),
		"the error path must cancel the context it created:\n"+failing)

	// A call that does not stream keeps the ordinary lifetime.
	plain := wrapperBody(t, pkg.Source, "crystallineFnSampleCancellable")

	testza.AssertTrue(t, strings.Contains(plain, "defer crystallineStop()"),
		"a call that ends when it returns must still cancel on the way out:\n"+plain)
}

func wrapperBody(t *testing.T, source string, name string) string {
	t.Helper()

	start := strings.Index(source, "func "+name)
	testza.AssertNotEqual(t, -1, start, "no wrapper named "+name)

	body := source[start:]

	return body[:strings.Index(body, "\n}\n")]
}

// TestSelfPackageBindingsCompile pins the layout the export directive implies:
// bindings generated into the same package as the code they bind.
//
// The qualifier is empty for the package the generated file belongs to, and it
// was concatenated with a dot regardless, so the call came out as ".Owned()".
// The shorthand documented for code you own had never worked in the layout it
// describes; every test reached the directive fixture through Build alone.
func TestSelfPackageBindingsCompile(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/directive"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	pkg, err := g.BuildGo(declarations, WithPackageName("directive"),
		WithImportPath("github.com/Vilsol/crystalline/testdata/directive"))
	testza.AssertNoError(t, err, "bindings generated into their own package must be valid Go")

	testza.AssertTrue(t, strings.Contains(pkg.Source, "r0 := Owned()"),
		"a call in the same package must not be qualified:\n"+pkg.Source)
	testza.AssertFalse(t, strings.Contains(pkg.Source, ":= .Owned()"),
		"the empty qualifier must not leave a stray dot:\n"+pkg.Source)
}

// TestSameNamedTypesStayDistinct pins that two packages may each declare a type
// of the same name.
//
// Generated identifiers were built from the bare Go name, so api.Config and
// db.Config produced one marshaller, called with both. format.Source only
// parses, so nothing noticed: BuildGo returned no error and an empty skip list,
// and the file it wrote failed to compile in the consumer's own build.
func TestSameNamedTypesStayDistinct(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/collide2/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	pkg, err := g.BuildGo(declarations, WithPackageName("main"), WithImportPath("example.com/main"))
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(pkg.Source, "(v *alpha.Config)"),
		"each package needs its own marshaller:\n"+pkg.Source)
	testza.AssertTrue(t, strings.Contains(pkg.Source, "(v *beta.Config)"),
		"including the second one:\n"+pkg.Source)

	// The names have to differ, or the file declares one function twice.
	testza.AssertEqual(t, 0, strings.Count(pkg.Source, "func crystallineMarshalConfig("),
		"the bare name is ambiguous:\n"+pkg.Source)
}

// BuildGo took the package name and import path as bare strings, and the
// command worked them out from the first manifest before every call. A library
// user had to know to do the same, and nothing said what happened if they
// guessed differently: the file names one package and the qualifier assumes
// another, which format.Source parses happily.
func TestBuildGoDefaultsToTheManifest(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/bindings/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	defaulted, err := g.BuildGo(declarations)
	testza.AssertNoError(t, err)

	spelled, err := g.BuildGo(declarations,
		WithPackageName("bindings"),
		WithImportPath("github.com/Vilsol/crystalline/testdata/bindings"))
	testza.AssertNoError(t, err)

	testza.AssertEqual(t, "bindings", defaulted.Package)
	testza.AssertEqual(t, spelled.Source, defaulted.Source,
		"the default must be what the manifest already says")
}

// Without a manifest there is nothing to default from, which is worth saying
// rather than writing a file whose package declaration is empty.
func TestBuildGoNeedsAPackageWithoutAManifest(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/directive"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	_, err = g.BuildGo(declarations)
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(errText(err), "WithPackageName"),
		"the error must name the option that fixes it, got: "+errText(err))
}

// Output could write its own files; the Go bindings could not, so every caller
// wrote the same MkdirAll and WriteFile.
func TestGoBindingsWriteFile(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/bindings/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	bindings, err := g.BuildGo(declarations)
	testza.AssertNoError(t, err)

	target := filepath.Join(t.TempDir(), "nested", "crystalline_gen.go")
	testza.AssertNoError(t, bindings.WriteFile(target))

	written, err := os.ReadFile(target)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, bindings.Source, string(written))
}

// bodyOf slices out one generated function, so an assertion about a wrapper
// cannot be satisfied by an unrelated part of the file.
func bodyOf(t *testing.T, source string, name string) string {
	t.Helper()

	start := strings.Index(source, "\nfunc "+name+"(")
	if start == -1 {
		// Fatal rather than reported: carrying on slices from -1 and panics,
		// which fails the whole binary and hides every test after it.
		t.Fatal(name + " is not in the generated source")
	}

	end := strings.Index(source[start+1:], "\nfunc ")
	if end == -1 {
		return source[start:]
	}

	return source[start : start+1+end]
}

// A conversion failure was reported by panicking, and crystallineRecover turned
// the panic back into the same thrown Error the wrapper could have returned
// itself. The round trip is free under the standard toolchain and fatal under
// TinyGo, whose wasm target has no recover: every one of these became
// RuntimeError: unreachable, taking the module with it.
//
// Cancellable is here because its conversion happens inside a promise body,
// which is the case a returned failure could get wrong by resolving.
func TestArgumentConversionsDoNotPanic(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	for _, name := range []string{"crystallineFnSampleMayFail", "crystallineFnSampleCancellable"} {
		body := bodyOf(t, bindings.Source, name)

		testza.AssertTrue(t, strings.Contains(body, "return crystallineFail(err.Error())"),
			name+" must report the failure rather than panic:\n"+body)
		testza.AssertFalse(t, strings.Contains(body, "crystallineMust("),
			name+" must convert its arguments without crystallineMust:\n"+body)
	}
}

// A field write has nowhere to return a value to, but it can report and stop.
func TestFieldWritesDoNotPanic(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	body := bodyOf(t, bindings.Source, "crystallineMarshalSampleRicher")

	testza.AssertTrue(t, strings.Contains(body, "crystallineFail(err.Error())"),
		"a field write must report the failure rather than panic:\n"+body)
	testza.AssertFalse(t, strings.Contains(body, "= crystallineMust("),
		"no field write may go through crystallineMust:\n"+body)
}

// Two places have to keep the panic, because the Go signature belongs to the
// consumer: what a JS callback returned, and what a method of a JS-supplied
// object returned. There is no error slot to return to, and inventing a zero
// value is the guess this project refuses to make.
func TestConsumerSignaturesStillPanic(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	for _, expected := range []string{
		"return crystallineMust(crystallineToInt(crystallineCallbackResult))",
		"return crystallineMust(crystallineToInt(crystallineSupplied))",
	} {
		testza.AssertTrue(t, strings.Contains(bindings.Source, expected),
			"missing "+expected+", which has nowhere else to report")
	}
}

// A field that cannot be written reported the refusal by panicking. It is the
// one refusal the move to reported failures missed, because it is emitted where
// the setter is built rather than where a conversion is.
func TestUnwritableFieldsDoNotPanic(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	body := bodyOf(t, bindings.Source, "crystallineMarshalSampleTicker")

	testza.AssertTrue(t, strings.Contains(body, "cannot be written from JavaScript"),
		"the refusal must still name the field and the reason:\n"+body)
	testza.AssertFalse(t, strings.Contains(body, "panic("),
		"the refusal must be reported rather than panicked:\n"+body)
}

// A field tagged bigint crosses as a JavaScript BigInt, which carries the value
// exactly. Reading one back cannot go through syscall/js: Value.Type() panics
// with "bad type flag" on a BigInt, and so does Value.Get, so the conversion
// asks JavaScript for the digits and parses them.
func TestBigIntFieldsCrossAsBigInt(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	body := bodyOf(t, bindings.Source, "crystallineMarshalSampleLedger")

	testza.AssertTrue(t, strings.Contains(body, "crystallineBigInt(int64(v.ID))"),
		"an int64 tagged bigint must be handed over as a BigInt:\n"+body)
	testza.AssertTrue(t, strings.Contains(body, "crystallineBigUint(uint64(v.Balance))"),
		"and a uint64 likewise:\n"+body)
	testza.AssertTrue(t, strings.Contains(body, "crystallineToBigInt(value)"),
		"a write must read the digits rather than the js.Value:\n"+body)
	testza.AssertTrue(t, strings.Contains(body, "float64(v.Rounded)"),
		"an untagged int64 must still be a number:\n"+body)
}

// The warning names every member carrying a 64-bit integer as a JavaScript
// number. A field that opted out of being one has nothing to warn about, and
// warning anyway is the same defect as any other pair of paths that disagree.
func TestBigIntFieldsAreNotWarnedAbout(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	var mentioned []string

	for _, warning := range bindings.Warnings {
		if strings.Contains(warning.Name, "Exact") {
			mentioned = append(mentioned, warning.Name+": "+warning.Reason)
		}
	}

	testza.AssertEqual(t, 0, len(mentioned),
		"a struct whose wide integers all carry bigint has nothing to warn about: "+strings.Join(mentioned, ", "))

	// And the warning still fires where a field did not opt out, or it would
	// have been switched off rather than made accurate.
	warned := false

	for _, warning := range bindings.Warnings {
		if strings.Contains(warning.Name, "Ledger") {
			warned = true
		}
	}

	testza.AssertTrue(t, warned, "Ledger.Rounded is an untagged int64 and must still be named")
}

// camelCase has to reach every place a Go name becomes a JavaScript one, and
// there are a dozen of them across the module, the declarations and the
// bindings. A spot check would pass while one of them still spelled the name
// the other way, which is the shape of every serious bug this project has had.
//
// So this asserts completeness rather than a sample: it collects every
// JS-visible member name out of the generated output and fails naming the ones
// that are still spelled in Go. Type names, namespaces, enum constants and the
// names given to r.Value are excluded deliberately — see WithCamelCase.
func TestCamelCaseReachesEveryName(t *testing.T) {
	g := NewGenerator("app", WithCamelCase())
	testza.AssertNoError(t, g.Load(".", "./testdata/bindings/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	rendered, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	bindings, err := g.BuildGo(declarations)
	testza.AssertNoError(t, err)

	capitalised := func(name string) bool {
		return name != "" && name[0] >= 'A' && name[0] <= 'Z'
	}

	var wrong []string

	// The bindings: every name published into the JS object graph.
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`crystallineDefine\(scope, out, "([^"]+)"`),
		regexp.MustCompile(`out\.Set\("([^"]+)"`),
		regexp.MustCompile(`\)\.Set\("([^"]+)", crystallineWrap`),
		regexp.MustCompile(`value\.Get\("([^"]+)"\)`),
		regexp.MustCompile(`c\.value\.Call\("([^"]+)"`),
	} {
		for _, found := range pattern.FindAllStringSubmatch(bindings.Source, -1) {
			if capitalised(found[1]) {
				wrong = append(wrong, "bindings: "+found[1])
			}
		}
	}

	// An enum's constants keep their Go names deliberately, and they render in
	// the same shape as an interface member, so they come out before the scan
	// rather than being excused inside it.
	enums := regexp.MustCompile(`(?s)  const \w+: \{.*?\n  \};\n`)
	declared := enums.ReplaceAllString(rendered.TypeScript, "")

	// The declarations: members of an interface, and exported functions.
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)^    (?:readonly )?([A-Za-z_$][\w$]*)[?]?(?:\(|:)`),
		regexp.MustCompile(`(?m)^  function ([A-Za-z_$][\w$]*)\(`),
	} {
		for _, found := range pattern.FindAllStringSubmatch(declared, -1) {
			if capitalised(found[1]) {
				wrong = append(wrong, "declarations: "+found[1])
			}
		}
	}

	slices.Sort(wrong)
	wrong = slices.Compact(wrong)

	testza.AssertEqual(t, 0, len(wrong),
		"these names are still spelled in Go:\n"+strings.Join(wrong, "\n"))
}

// The module reads each binding back out of the JavaScript object graph by
// name, and the bindings put it there by name. Those are two computations of
// one string in two files, which is the shape of every serious bug this project
// has had: renaming for camelCase changed one of them and left the module
// reading a property that no longer existed.
//
// So rather than checking the spelling, this checks that they agree.
func TestModuleReadsWhatTheBindingsPublish(t *testing.T) {
	for _, spelling := range []struct {
		name    string
		options []GeneratorOption
	}{
		{name: "go names"},
		{name: "camelCase", options: []GeneratorOption{WithCamelCase()}},
	} {
		g := NewGenerator("app", spelling.options...)
		testza.AssertNoError(t, g.Load(".", "./testdata/bindings/..."))

		declarations, err := g.Declarations()
		testza.AssertNoError(t, err)

		rendered, err := g.Build(declarations)
		testza.AssertNoError(t, err)

		bindings, err := g.BuildGo(declarations)
		testza.AssertNoError(t, err)

		published := make(map[string]bool)
		for _, found := range regexp.MustCompile(`\)\.Set\("([^"]+)"`).FindAllStringSubmatch(bindings.Source, -1) {
			published[found[1]] = true
		}

		testza.AssertTrue(t, len(published) > 0, spelling.name+": nothing was published")

		// Every property the module reads off a namespace object.
		read := regexp.MustCompile(`\['app'\]\['\w+'\]\['([^']+)'\]`)

		var missing []string

		for _, found := range read.FindAllStringSubmatch(rendered.JavaScript, -1) {
			if !published[found[1]] {
				missing = append(missing, found[1])
			}
		}

		slices.Sort(missing)

		testza.AssertEqual(t, 0, len(missing),
			spelling.name+": the module reads names the bindings never published: "+strings.Join(slices.Compact(missing), ", "))
	}
}

// camelNaming is the surface a camelCase build presents, checked through the
// generated module rather than through the object graph beneath it.
//
// The static tests prove the module and the bindings agree about every name.
// This proves the names work: the module is imported, the binary is booted the
// way a page boots it, and every kind of member is called by its new name.
const camelNaming = `import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";

// wasm_exec.js is a plain script that defines globalThis.Go.
createRequire(import.meta.url)("./wasm_exec.js");

const loaded = await import("./crystalline.js");
const { sample } = await loaded.boot(await readFile(new URL("./app.wasm", import.meta.url)));

const out = [];

// A function, and the Go spelling gone rather than there as well.
out.push("fn=" + sample.basic());
out.push("goneFn=" + (sample.Basic === undefined));

// A field and a method on a live wrapper.
const wrapper = sample.fooBar();
out.push("field=" + wrapper.firstValue);
out.push("method=" + wrapper.a(true));
out.push("goneField=" + (wrapper.FirstValue === undefined));

// A write has to reach the Go value under the new name too.
wrapper.firstValue = "written";
out.push("write=" + wrapper.one());

// An object literal is validated against the names it now has.
const rich = sample.rich();
out.push("literal=" + rich.configure({ firstValue: "literal" }));

let staleKey = "accepted";
try {
	rich.configure({ FirstValue: "stale" });
} catch (error) {
	staleKey = error.message.includes("unknown property") ? "threw" : "wrong:" + error.message;
}
out.push("staleKey=" + staleKey);

// An object JavaScript supplies is written in JavaScript, so Go calls out
// through the new names as well.
const recorded = [];
const level = await sample.replay({
	record: (event) => { recorded.push(event); },
	level: () => 7,
}, ["a", "b"]);
out.push("supplied=" + level + ":" + recorded.join(","));

// An acronym lowers as a whole, and an explicit name wins over the convention.
const ledger = sample.newLedger();
out.push("acronym=" + ledger.id);
out.push("renamed=" + ledger.memo);

console.log(out.join(" | "));

process.exit(0);
`

// buildCamelModule generates the whole surface with camelCase on and assembles
// a directory a page could load: the binary, the module and the runtime shim.
func buildCamelModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	repo, err := filepath.Abs(".")
	testza.AssertNoError(t, err)

	g := NewGenerator("app", WithCamelCase())
	testza.AssertNoError(t, g.Load(".", "./testdata/bindings/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	rendered, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	bindings, err := g.BuildGo(declarations, WithPackageName("main"), WithImportPath("bindtest"))
	testza.AssertNoError(t, err)

	shim := filepath.Join(strings.TrimSpace(runGo(t, "env", "GOROOT")), "lib", "wasm", "wasm_exec.js")

	script, err := os.ReadFile(shim)
	testza.AssertNoError(t, err, "the Go wasm shim must be readable at "+shim)

	sum, err := os.ReadFile("go.sum")
	testza.AssertNoError(t, err)

	gomod := "module bindtest\n\ngo 1.27\n\nrequire github.com/Vilsol/crystalline v0.0.0\n\nreplace github.com/Vilsol/crystalline => " + repo + "\n"

	for name, content := range map[string]string{
		"crystalline_gen.go": bindings.Source,
		"crystalline.js":     rendered.JavaScript,
		"wasm_exec.js":       string(script),
		"run.mjs":            camelNaming,
		"go.mod":             gomod,
		"go.sum":             string(sum),
		// The bindings register themselves, so the program only has to stay
		// alive for JavaScript to call into.
		"main.go": "package main\n\nfunc main() {\n\tselect {}\n}\n",
	} {
		testza.AssertNoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	goCommand(t, dir, "mod", "tidy")
	goCommand(t, dir, "build", "-ldflags=-s -w", "-o", "app.wasm", ".")

	return dir
}

func TestCamelCaseNamesWorkAtRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a wasm binary and runs it under node")
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}

	dir := buildCamelModule(t)

	cmd := exec.Command(node, filepath.Join(dir, "run.mjs"))
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

	output, err := cmd.CombinedOutput()
	testza.AssertNoError(t, err, string(output))

	reported := probeResults(t, string(output))

	for _, expected := range []string{
		"fn=420",
		"goneFn=true",
		"field=hello",
		"method=true",
		"goneField=true",
		"write=written",
		"literal=literal",
		// The unknown-property check has to know the new names, or a stale one
		// would be accepted and silently zeroed.
		"staleKey=threw",
		// Go calls out through the names JavaScript supplied.
		"supplied=7:a,b",
		// ID lowers as a whole rather than becoming iD.
		"acronym=9007199254740993",
		// And name= wins over the convention.
		"renamed=exact",
	} {
		key, value, _ := strings.Cut(expected, "=")
		testza.AssertEqual(t, value, reported[key], key+" was wrong in:\n"+string(output))
	}
}

// The identifier a converter is named after and the name a person reads are two
// different things, and one string was doing both. The identifier carries the
// package so that two packages may each declare a Config, which makes it
// SampleFnSample — a name that appears nowhere in the Go and cannot be searched
// for.
func TestConverterMessagesNameTheGoType(t *testing.T) {
	bindings, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	body := bodyOf(t, bindings.Source, "crystallineToSampleFnSample")

	for _, expected := range []string{
		`"sample.FnSample: expected an object"`,
		`"sample.FnSample: expected an object, got null"`,
		`"sample.FnSample: the value behind this handle has been released"`,
		`crystallineUnknownProperty(value, "sample.FnSample"`,
	} {
		testza.AssertTrue(t, strings.Contains(body, expected),
			"missing "+expected+" in:\n"+body)
	}

	testza.AssertFalse(t, strings.Contains(body, `"SampleFnSample:`),
		"no message may use the generated identifier:\n"+body)

	// The identifier itself still carries the package, or two packages each
	// declaring a type of one name would share a converter.
	testza.AssertTrue(t, strings.Contains(bindings.Source, "crystallineKnownSampleFnSample"),
		"the identifier must stay unique across packages")
}
