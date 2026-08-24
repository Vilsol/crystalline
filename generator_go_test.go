//go:build !js

package crystalline

import (
	"os"
	"os/exec"
	"path/filepath"
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

	return g.BuildGo(declarations, "main", "bindtest")
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
			callbackBad = "threw";
		}
		out.push("CallbackBad=" + callbackBad);

		// The string case is the one that was silent: Go received the literal
		// "<undefined>" and carried on.
		let textBad = "accepted";
		try {
			await r.WithText((v) => {});
		} catch (e) {
			textBad = "threw";
		}
		out.push("TextBad=" + textBad);
		out.push("TextGood=" + await r.WithText((v) => v + "!"));

		// A struct parameter is not optional: null must not arrive in Go as a
		// zero value nobody asked for.
		let nullStruct = "accepted";
		try {
			r.Configure(null);
		} catch (e) {
			nullStruct = "threw";
		}
		out.push("NullStruct=" + nullStruct);

		// Every typed array has a byteLength, so checking for one accepted any
		// of them and then copied nothing.
		let wrongBytes = "accepted";
		try {
			r.Apply({Blob: new Int32Array([1, 2, 3])});
		} catch (e) {
			wrongBytes = "threw";
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

		// A wrapper handed back to Go must resolve to the same Go value.
		out.push("StructIdentity=" + r.Configure(live));

		// A plain object literal must still be accepted.
		out.push("StructLiteral=" + r.Configure({FirstValue: "literal"}));

		// A misspelled property must be rejected, not silently zeroed.
		let rejected = "no";
		try {
			r.Configure({FirstVlaue: "typo"});
		} catch (e) {
			rejected = "yes";
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
			badTime = "threw";
		}
		out.push("BadTime=" + badTime);

		// A declared mapping: Colour crosses as a string, both ways.
		const m = globalThis.go.app.marshal;
		out.push("Colour=" + m.Brighten("#102030"));
		let badColour = "accepted";
		try {
			m.Brighten("nonsense");
		} catch (e) {
			badColour = "threw";
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
			missingMethod = "threw";
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
			badFeed = "threw";
		}
		out.push("BadFeed=" + badFeed);

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

	pkg, err := g.BuildGo(declarations, "directive", "github.com/Vilsol/crystalline/testdata/directive")
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

	pkg, err := g.BuildGo(declarations, "main", "example.com/main")
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(pkg.Source, "(v *alpha.Config)"),
		"each package needs its own marshaller:\n"+pkg.Source)
	testza.AssertTrue(t, strings.Contains(pkg.Source, "(v *beta.Config)"),
		"including the second one:\n"+pkg.Source)

	// The names have to differ, or the file declares one function twice.
	testza.AssertEqual(t, 0, strings.Count(pkg.Source, "func crystallineMarshalConfig("),
		"the bare name is ambiguous:\n"+pkg.Source)
}
