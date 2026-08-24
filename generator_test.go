//go:build !js

package crystalline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarvinJWendt/testza"
)

// TestGeneratedDeclarationsMatchGolden pins the emitted declarations and module
// against checked-in expectations.
//
// It replaces the parity test that compared against the reflect runtime: with
// that path gone there is nothing to compare to, so the output is pinned
// directly. Run with UPDATE_GOLDEN=1 to rewrite the files after an intended
// change, then read the diff.
func TestGeneratedDeclarationsMatchGolden(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	assertGolden(t, "sample.d.ts", out.TypeScript)
	assertGolden(t, "sample.js", out.JavaScript)
}

// TestGeneratorNamesCallbackParameters pins a place reflection could not reach:
// the parameters of a func-typed parameter.
func TestGeneratorNamesCallbackParameters(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "cb: (v: string) => number | PromiseLike<number>"),
		"callback parameter names must survive:\n"+out.TypeScript)
}

// TestGeneratorReadsPromiseDirectiveStatically is the point of the exercise: the
// directive is recovered from parsed source rather than from a file read at run
// time, so it cannot silently degrade in a deployed binary.
func TestGeneratorReadsPromiseDirectiveStatically(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "C(x: boolean): Promise<boolean>;"),
		"crystalline:promise must be honoured:\n"+out.TypeScript)
}

// TestGeneratorRecoversParameterNames shows the runtime AST hack falling away:
// go/types carries parameter names.
func TestGeneratorRecoversParameterNames(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "A(x: boolean)"),
		"parameter names must survive:\n"+out.TypeScript)
}

func TestGeneratorReportsLoadFailures(t *testing.T) {
	g := NewGenerator("app")

	testza.AssertNotNil(t, g.Load(".", "./testdata/does-not-exist"))
}

func staticBuild(t *testing.T) (Output, error) {
	t.Helper()

	g := NewGenerator("app")
	if err := g.Load(".", "./testdata/bindings"); err != nil {
		return Output{}, err
	}

	declarations, err := g.Declarations()
	if err != nil {
		return Output{}, err
	}

	return g.Build(declarations)
}

// assertGolden compares against a checked-in file, rewriting it when
// UPDATE_GOLDEN is set.
func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name)

	if os.Getenv("UPDATE_GOLDEN") != "" {
		testza.AssertNoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		testza.AssertNoError(t, os.WriteFile(path, []byte(actual), 0o644))

		return
	}

	expected, err := os.ReadFile(path)
	testza.AssertNoError(t, err, "missing golden file; re-run with UPDATE_GOLDEN=1")

	testza.AssertEqual(t, string(expected), actual)
}

// TestErrorReturnsBecomeResults pins the calling convention for failure.
//
// A trailing error says a call can fail, not that it is slow. Making it a
// promise forces every caller to be async, which spreads through a codebase for
// no reason, so failure is carried by a Result the caller narrows instead.
func TestErrorReturnsBecomeResults(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function MayFail(ok: boolean): Result<string>;"),
		"a (T, error) return must stay synchronous:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function OnlyFails(ok: boolean): Result<void>;"),
		"an error-only return must stay synchronous:\n"+out.TypeScript)

	// One type, not a union: a caller should reach for unwrap, not narrow at
	// every call site.
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "export interface Result<T> {"),
		"Result must be a single interface:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "Ok<T> | Err"),
		"Result must not be a union:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "unwrap(): T;"), out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "unwrapOr(fallback: T): T;"), out.TypeScript)
}

// TestAsyncErrorReturnsStayResults pins that being asynchronous and being able
// to fail stay separate: an explicit promise carries a Result, so there is one
// way to handle a Go error whatever the call's timing.
func TestAsyncErrorReturnsStayResults(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Cancellable(signal: AbortSignal, label: string): Promise<Result<string>>;"),
		"an async call that can fail must be Promise<Result<T>>:\n"+out.TypeScript)
}

// TestChannelsBecomeAsyncIterables pins the second translation: a receive-only
// channel and an async iterator mean the same thing, and refusing channels
// outright leaves a Go library unable to express a stream at all.
func TestChannelsBecomeAsyncIterables(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Stream(count: number): AsyncIterable<string>;"),
		"a receive-only channel must become an AsyncIterable:\n"+out.TypeScript)
}

// TestContextBecomesAbortSignal pins the third: a Go call blocks the single JS
// thread with no way to cancel it unless the context is reachable.
func TestContextBecomesAbortSignal(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "Cancellable(signal: AbortSignal, label: string)"),
		"a leading context must surface as an AbortSignal:\n"+out.TypeScript)
}

// TestChannelParametersAcceptIterables pins the mirror of a streaming return: a
// channel the caller fills is the same idea as one it drains, so it takes the
// same type in the other direction.
func TestChannelParametersAcceptIterables(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Sum(values: AsyncIterable<number>): Promise<number>;"),
		"a channel parameter must accept an iterable:\n"+out.TypeScript)
}

// TestBidirectionalChannelReturnIsAccepted pins that a returned channel needs
// no annotation: whatever the type says, the caller can only read it.
func TestBidirectionalChannelReturnIsAccepted(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/bidimanifest"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Made(): AsyncIterable<string>;"), out.TypeScript)
}

// TestBidirectionalChannelParameterIsRejected pins that an undirected parameter
// is refused rather than guessed at. Reading it as a source silently discards
// anything the function sends, and a duplex over a single channel cannot work:
// Go would receive its own values.
//
// Refused means left out of every artifact and named in the report, not the
// whole build failing: one member nobody can bind should not stop the rest of a
// package from being generated.
func TestBidirectionalChannelParameterIsRejected(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/bidiparam"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	testza.AssertFalse(t, strings.Contains(out.TypeScript, "Both"),
		"an undirected channel parameter must not be declared:\n"+out.TypeScript)

	reported := make([]string, 0, len(out.Skipped))
	for _, skipped := range out.Skipped {
		reported = append(reported, skipped.String())
	}

	joined := strings.Join(reported, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "Both"),
		"it must be named, got:\n"+joined)
	testza.AssertTrue(t, strings.Contains(joined, "<-chan"),
		"and the report must say how to fix it, got:\n"+joined)
}

// TestPointerParametersStayRequired pins that a pointer parameter renders as a
// union rather than an optional parameter.
//
// A pointer means null is allowed, not that the argument may be omitted, and
// TypeScript rejects a required parameter that follows an optional one, so the
// question-mark form produced a declaration file that would not compile.
func TestPointerParametersStayRequired(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Middle(first: sample.FnSample | undefined, label: string): string;"),
		"a pointer parameter must stay required:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "first?:"),
		"a pointer parameter must not be marked optional:\n"+out.TypeScript)
}

// TestNilableSliceParametersStayRequired covers the shape a consumer hit: a
// slice can be nil, but a nil slice is a value rather than an absent argument,
// so declaring it optional made the required parameter after it illegal.
func TestNilableSliceParametersStayRequired(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Keys(ids: Array<number> | undefined, seed: number): number;"),
		"a nilable slice parameter must stay required:\n"+out.TypeScript)
}

// TestPlainTypesAreDeclaredAsData pins what r.Plain changes in the
// declarations: fields that cannot be written back, and no methods, because
// plain data is a snapshot rather than a view of the Go value.
func TestPlainTypesAreDeclaredAsData(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "readonly Label: string;"),
		"a plain field must be readonly:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "readonly At: string;"),
		"plainness must reach the structs a plain type contains:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "Describe(): string;"),
		"plain data carries no methods:\n"+out.TypeScript)
}

// TestBuildReportsSkips pins that a caller who only builds the declarations
// still learns what could not be bound. The report used to be reachable from
// BuildGo alone, so a library user rendering types saw nothing.
func TestBuildReportsSkips(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	reported := make([]string, 0, len(out.Skipped))
	for _, skipped := range out.Skipped {
		reported = append(reported, skipped.String())
	}

	joined := strings.Join(reported, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "Sample.Describe"),
		"a method dropped by plain marshalling must be named, got:\n"+joined)
}

// TestValueNamespacesAgreeAcrossArtifacts pins that a value is published under
// one namespace, decided once.
//
// A consumer reported the declarations and the module disagreeing about a map
// keyed by a named type, which would typecheck and then be undefined at run
// time. All three artifacts are checked together here so that they cannot drift
// apart, and so that the namespace follows the type rather than the manifest.
func TestValueNamespacesAgreeAcrossArtifacts(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "const Titles: Record<number, string>"),
		"the value must be declared:\n"+out.TypeScript)

	declared := namespaceOf(t, out.TypeScript, "export declare namespace ", " {", "Titles")
	bound := namespaceOf(t, out.JavaScript, "  ", " = {", "Titles")

	testza.AssertEqual(t, "sample", declared,
		"a map keyed by a named type belongs with that package:\n"+out.TypeScript)
	testza.AssertEqual(t, declared, bound,
		"the declarations and the module must agree:\n"+out.JavaScript)

	pkg, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(pkg.Source, `crystallineNamespace("app", "`+declared+`").Set("Titles"`),
		"the bindings must publish it where the declarations say:\n"+pkg.Source)
}

// namespaceOf finds which namespace block a member was rendered into.
func namespaceOf(t *testing.T, source string, opener string, closer string, member string) string {
	t.Helper()

	current := ""

	for _, line := range strings.Split(source, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(line, opener) && strings.HasSuffix(trimmed, strings.TrimSpace(closer)) {
			current = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, strings.TrimSpace(opener)), strings.TrimSpace(closer)))
		}

		if strings.Contains(line, member+":") {
			return current
		}
	}

	return ""
}

// TestStreamingCallsAreNotWrappedInAPromise pins that a call returning a
// channel stays synchronous even when it takes a context.
//
// The stream is already asynchronous, so wrapping it made the caller write
// "for await (const x of await f())": an await whose only purpose was to unwrap
// something that had nothing to wait for.
func TestStreamingCallsAreNotWrappedInAPromise(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Ticks(signal: AbortSignal, count: number): AsyncIterable<number>;"),
		"a cancellable stream must not be a promise:\n"+out.TypeScript)
}

// TestResultIsDeclaredOnlyWhenUsed pins that a surface with nothing fallible
// does not carry the Result declaration, which would be a type a consumer can
// name but never receive.
func TestResultIsDeclaredOnlyWhenUsed(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/crosspkg/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	testza.AssertFalse(t, strings.Contains(out.TypeScript, "interface Result"),
		"nothing here can fail, so Result must not be declared:\n"+out.TypeScript)

	// The sample surface does have fallible calls, so it must still carry it.
	used, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(used.TypeScript, "export interface Result<T> {"),
		"a fallible surface must declare Result:\n"+used.TypeScript)
}

// TestUnbindableMembersAreSkippedNotFatal pins one policy for something that
// cannot be bound: it is reported and left out of every artifact.
//
// The two builders disagreed. BuildGo skipped and reported; Build errored for a
// channel and silently declared an interface{} as unknown. So the command could
// not generate a package containing one unbindable member at all, and where it
// did generate, the declarations described functions the bindings never
// published, giving wrap(undefined) and a TypeError at the call.
func TestUnbindableMembersAreSkippedNotFatal(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/nobind/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err, "one unbindable member must not stop the whole build")

	testza.AssertFalse(t, strings.Contains(out.TypeScript, "Send"),
		"an unbindable member must not be declared:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.JavaScript, "Send"),
		"an unbindable member must not be bound:\n"+out.JavaScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "Fine"),
		"its bindable siblings must survive:\n"+out.TypeScript)

	reported := make([]string, 0, len(out.Skipped))
	for _, skipped := range out.Skipped {
		reported = append(reported, skipped.String())
	}

	testza.AssertTrue(t, strings.Contains(strings.Join(reported, "\n"), "Send"),
		"and it must be named, got:\n"+strings.Join(reported, "\n"))
}

// TestWideIntegersAreWarnedAbout pins the one place crystalline knowingly loses
// information rather than refusing.
//
// A JavaScript number is a double, so an int64 past 2^53 is silently rounded.
// Refusing the type would break ordinary Go, since identifiers and timestamps
// are routinely int64, so it is bound and the cost is stated at generate time.
func TestWideIntegersAreWarnedAbout(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Big(n: number): number;"),
		"a 64-bit integer is still bound as a number:\n"+out.TypeScript)

	reported := make([]string, 0, len(out.Warnings))
	for _, warning := range out.Warnings {
		reported = append(reported, warning.String())
	}

	joined := strings.Join(reported, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "sample.Big"),
		"the member must be named, got:\n"+joined)
	testza.AssertTrue(t, strings.Contains(joined, "int64"),
		"and the type responsible, got:\n"+joined)
	testza.AssertTrue(t, strings.Contains(joined, "2^53"),
		"and why it matters, got:\n"+joined)
}

// TestWrappersDeclareTheirDisposer pins that a live wrapper says it holds
// resources.
//
// The runtime sets release and Symbol.dispose on every wrapper, the README
// advertises "using config = api.LoadConfig()", and the declarations mentioned
// neither: our own example called release() on a type that did not declare it.
// Plain data holds nothing, so it must not claim to.
func TestWrappersDeclareTheirDisposer(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	wrapper := interfaceBlock(t, out.TypeScript, "FnSample")

	testza.AssertTrue(t, strings.Contains(wrapper, "release(): void;"),
		"a wrapper must declare its disposer:\n"+wrapper)
	testza.AssertTrue(t, strings.Contains(wrapper, "[Symbol.dispose](): void;"),
		"and the one a using-declaration calls:\n"+wrapper)

	plain := interfaceBlock(t, out.TypeScript, "Reading")

	testza.AssertFalse(t, strings.Contains(plain, "release"),
		"plain data holds nothing to release:\n"+plain)
}

// interfaceBlock returns the body of one declared interface.
func interfaceBlock(t *testing.T, source string, name string) string {
	t.Helper()

	start := strings.Index(source, "  interface "+name+" {")
	testza.AssertNotEqual(t, -1, start, "no interface named "+name+" in:\n"+source)

	body := source[start:]

	return body[:strings.Index(body, "\n  }")]
}

// TestEmbeddedMembersArePromoted pins Go's promotion rules reaching JavaScript.
//
// The method set was read with NumMethods, which is declared-only, and the
// embedded field was dropped as well, so a struct that embeds another lost part
// of its surface with nothing reported. Embedding is everywhere in Go.
func TestEmbeddedMembersArePromoted(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	block := interfaceBlock(t, out.TypeScript, "Embedder")

	// The embedded value stays addressable under its own name rather than being
	// flattened: promotion would need shadowing rules, and this needs none.
	testza.AssertTrue(t, strings.Contains(block, "Base: sample.Base;"),
		"an embedded field must be reachable:\n"+block)
	testza.AssertTrue(t, strings.Contains(block, "Promoted(): string;"),
		"and so must its methods:\n"+block)
	testza.AssertTrue(t, strings.Contains(block, "Direct(): string;"),
		"the type's own members must survive too:\n"+block)
}

// TestTimeIsMarshalledAsADate pins the stdlib mapping that r.Marshal provides.
//
// time.Time has no exported fields, so it used to bind as a wrapper carrying
// thirty methods and no readable data, and its converter had an empty set of
// known fields. That meant the unknown-property check had nothing to reject:
// a real JS Date has no own enumerable keys, so it was accepted and silently
// became the zero time.
func TestTimeIsMarshalledAsADate(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function TakesTime(t: Date): string;"),
		"a time must cross as a Date:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "At: Date;"),
		"including as a field:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "namespace time"),
		"and its methods must not be declared:\n"+out.TypeScript)
}

// TestCustomMarshallerMapsAType pins r.Marshal: a project maps one of its own
// types onto a JavaScript counterpart with a pair of ordinary Go functions.
//
// The signatures carry the whole declaration. func(Colour) string says Colour
// crosses as a string, and func(string) (Colour, error) says how it comes back
// and that it can refuse.
func TestCustomMarshallerMapsAType(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/marshalmanifest/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Brighten(c: string): string;"),
		"the mapped type must cross as its counterpart:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "interface Colour"),
		"and must not also be declared as a struct:\n"+out.TypeScript)
}

// TestDiagnosticsCarryTheirPosition pins that a report says where in the source
// the problem is.
//
// go/packages hands over a position for every symbol and the file set to render
// it against, and both were being thrown away, so a skip named a symbol and
// left the reader to find it. Editors and CI annotate a file:line:col prefix
// without being taught anything.
func TestDiagnosticsCarryTheirPosition(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/nobind/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, len(out.Skipped) > 0, "the fixture must produce a skip")

	reported := out.Skipped[0].String()

	// The position is where the symbol is declared, not where the manifest
	// mentioned it, which is the one a reader has to go and edit.
	testza.AssertTrue(t, strings.Contains(reported, "testdata/unbindable/unbindable.go:"),
		"a skip must name the file it came from, got: "+reported)
	testza.AssertFalse(t, strings.HasPrefix(reported, "/"),
		"and relative to the working directory, got: "+reported)
	testza.AssertTrue(t, strings.Contains(reported, "Send"),
		"and still name the symbol, got: "+reported)
}

// TestEnumsKeepTheirNames pins that a named integer type with a fixed set of
// constants arrives as more than a bare number.
//
// Go writes an enum as a named type plus a const block plus a String method,
// and all three were collapsed to "number": the constants, their names and
// their text were unreachable from JavaScript.
func TestEnumsKeepTheirNames(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "type Phase = 0 | 1 | 2;"),
		"the value set must be declared:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "const Phase: {"),
		"the constants must be reachable by name:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Advance(p: sample.Phase): sample.Phase;"),
		"and the type must be used where it appears:\n"+out.TypeScript)
}

// TestModuleExportsALoader pins that the module can start itself.
//
// Every project hand-wrote the same six lines — construct the runtime, fetch,
// instantiate, run without awaiting, initialise — and the pending() proxy
// exists only to explain what happens when that sequence is got wrong. A
// loader that returns the namespaces removes the ordering hazard rather than
// reporting it.
func TestModuleExportsALoader(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.JavaScript, "export const boot"),
		"the module must export a loader:\n"+out.JavaScript)
	testza.AssertTrue(t, strings.Contains(out.JavaScript, "initializeCrystalline()"),
		"which initialises for you:\n"+out.JavaScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "export function boot(wasm: string | URL | BufferSource): Promise<{"),
		"and is declared with what it hands back:\n"+out.TypeScript)
}

// TestProfilingCountsCrossings pins the opt-in counter.
//
// The documentation's main advice is to count crossings rather than worry about
// conversions, and nothing counted them: the finding that 98 calls cost half a
// millisecond was arrived at by hand. Off by default, because a counter on a
// five microsecond call is not free.
func TestProfilingCountsCrossings(t *testing.T) {
	plain, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertFalse(t, strings.Contains(plain.JavaScript, "export const stats"),
		"profiling must be off by default:\n"+plain.JavaScript)

	g := NewGenerator("app", WithProfiling())
	testza.AssertNoError(t, g.Load(".", "./testdata/bindings"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.JavaScript, "export const stats"),
		"profiling must expose what it counted:\n"+out.JavaScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "export function stats():"),
		"and declare it:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.JavaScript, "performance.now()"),
		"and time each crossing:\n"+out.JavaScript)
}

// TestInterfaceParametersComeFromJS pins the narrow form of the other
// direction: Go declares an interface, JavaScript supplies an object with those
// methods.
//
// It reuses what a callback parameter already does, one method at a time, so it
// is a bundle of existing pieces rather than a second generator for binding
// arbitrary browser APIs.
func TestInterfaceParametersComeFromJS(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Replay(r: sample.Recorder, events: Array<string> | undefined): Promise<number>;"),
		"an interface parameter must take the declared type:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "interface Recorder {"),
		"which must be declared:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "Record(event: string): void;"),
		"with the methods it needs:\n"+out.TypeScript)
}

// TestComplexIsRefusedByBothPaths pins that the two builders agree about a type
// with no JavaScript counterpart.
//
// types.IsNumeric includes complex, so the converter accepted it and emitted
// complex128(value.Float()), while the declarations refused it outright. The
// command runs Build first, so a package containing one complex parameter
// generated nothing at all and reported an error rather than a skip.
func TestComplexIsRefusedByBothPaths(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/nobind/..."))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err, "one unbindable parameter must not stop the build")

	reported := make([]string, 0, len(out.Skipped))
	for _, skipped := range out.Skipped {
		reported = append(reported, skipped.String())
	}

	joined := strings.Join(reported, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "Scale"),
		"a complex parameter must be reported, got:\n"+joined)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "Scale"),
		"and left out of the declarations:\n"+out.TypeScript)
}

// TestEnumsAreFoundAcrossPackages pins that a type declared elsewhere still
// arrives as an enum.
//
// The packages to load were chosen with the same walk that decides what to
// declare, and that walk asks whether a type has constants — which cannot be
// answered before the package holding them is loaded. So a package reached only
// through another package's signature was never loaded, its constants were
// never found, and its enum quietly became a number.
func TestEnumsAreFoundAcrossPackages(t *testing.T) {
	g := NewGenerator("app")

	// Only the manifest is a root: inner is reached through outer's signature.
	testza.AssertNoError(t, g.Load(".", "./testdata/crosspkg/manifest"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "type Mode = 0 | 1;"),
		"an enum from a package reached indirectly must still be one:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Switch(m: inner.Mode): inner.Mode;"),
		"and be used where it appears:\n"+out.TypeScript)
}

// TestPointersToMappedTypesAreMapped pins that a pointer honours its element's
// mapping.
//
// The pointer case reached for the struct marshaller before asking whether the
// type had a mapping, so *time.Time produced the thirty-method live wrapper
// that mapping time.Time exists to avoid, and the declarations described a
// shape the bindings never published.
func TestPointersToMappedTypesAreMapped(t *testing.T) {
	out, err := staticBuild(t)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function MaybeStamp(ok: boolean): (Date | undefined);"),
		"a pointer to a mapped type must cross as its counterpart:\n"+out.TypeScript)
	testza.AssertFalse(t, strings.Contains(out.TypeScript, "namespace time"),
		"and must not drag in the type it replaced:\n"+out.TypeScript)
}

// TestPointersToMappedTypesBindAsMapped is the same claim about the bindings.
//
// The declarations were already right, so checking them alone would have missed
// this: the Go called the wrapper marshaller, which is both the wrong shape and
// the thirty bound methods the mapping exists to avoid.
func TestPointersToMappedTypesBindAsMapped(t *testing.T) {
	pkg, err := generateBindings(t, "./testdata/bindings")
	testza.AssertNoError(t, err)

	testza.AssertFalse(t, strings.Contains(pkg.Source, "crystallineMarshalTimeTime"),
		"a mapped type must not also get a wrapper marshaller:\n"+pkg.Source)
}
