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

	output := runWasm(t, dir)

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
		// Assigning a field from JS must reach the Go value behind it.
		"WriteThrough=written",
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
		"SumArray=6",
		"SumAsync=30",
		"First=1",
		"FeedStopped=true",
		// Cancellation.
		"Cancellable=live",
		"Aborted=yes",
		// Explicit disposal.
		"Disposed=yes",
	} {
		testza.AssertTrue(t, strings.Contains(output, expected),
			"missing "+expected+" in:\n"+output)
	}
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
		const live = s.FooBar();
		live.FirstValue = "written";
		out.push("WriteThrough=" + live.One());

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

		out.push("Cancellable=" + (await s.Cancellable(undefined, "live")).unwrap());

		// A channel parameter accepts anything iterable.
		out.push("SumArray=" + await s.Sum([1, 2, 3]));
		async function* generated() { yield 10; yield 20; }
		out.push("SumAsync=" + await s.Sum(generated()));

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
		js.Global().Get("console").Call("log", "REJECTED: "+args[0].String())
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
