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

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "cb: (v: string) => Promise<number>"),
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
func TestBidirectionalChannelParameterIsRejected(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/bidiparam"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	_, err = g.Build(declarations)
	testza.AssertNotNil(t, err, "an undirected channel parameter must not be guessed at")
	testza.AssertTrue(t, strings.Contains(err.Error(), "<-chan"),
		"the error must say how to fix it, got: "+errText(err))
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
