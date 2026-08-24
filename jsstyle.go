//go:build !js

package crystalline

import (
	"fmt"
	"os"
	"path/filepath"
)

// jsStyle holds the cosmetic choices for the emitted JavaScript. It lives on
// the Generator rather than in package globals so that two generators in one
// process can differ.
type jsStyle struct {
	quote         string
	trailingComma bool
}

func defaultStyle() jsStyle {
	return jsStyle{quote: "'"}
}

// quoted renders a value as a JavaScript string literal in the configured style.
func (s jsStyle) quoted(value string) string {
	return s.quote + value + s.quote
}

// jsWrapHelper turns the error slot the Go side writes into a thrown JS Error.
const jsWrapHelper = `const wrap = (fn) => {
  return (...args) => {
    const result = fn.call(undefined, ...args);
    if (globalThis.goInternalError) {
      const error = new Error(globalThis.goInternalError);
      globalThis.goInternalError = undefined;
      throw error;
    }
    return result;
  }
};`

// initGuard fails loudly when the module has not started, instead of letting
// the caller trip over an undefined property.
func initGuard(style jsStyle, appName string) string {
	return "  if (globalThis[" + style.quoted("go") + "]?.[" + style.quoted(appName) + "] === undefined) {\n" +
		"    throw new Error(" + style.quoted("crystalline: globalThis.go."+appName+" is not set. Start the Go wasm module before calling initializeCrystalline().") + ");\n" +
		"  }\n\n"
}

// WriteFiles writes the generated sources to the given paths, creating parent
// directories as needed.
func (o Output) WriteFiles(jsPath string, tsPath string) error {
	for target, content := range map[string]string{jsPath: o.JavaScript, tsPath: o.TypeScript} {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", target, err)
		}

		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
	}

	return nil
}

// orUndefined marks a value JavaScript may not have.
const orUndefined = " | undefined"

// Output is the pair of files a build produces.
type Output struct {
	// JavaScript is the ES module that binds the wasm exports into a
	// namespaced object graph. Import it and call initializeCrystalline()
	// once the wasm module is running.
	JavaScript string

	// TypeScript is the matching .d.ts declaration file.
	TypeScript string

	// Skipped lists everything that could not be bound, so a caller that only
	// builds the declarations still learns about the gaps.
	Skipped []Skipped
}

// resultDeclarations declares the shape a fallible call returns.
//
// A Go error says a call can fail, not that it is slow, so failure travels in
// the value rather than in a rejected promise: making every fallible call async
// would force every caller to be async too.
//
// It is one interface rather than a discriminated union. A union is only
// pleasant when you actually want to branch, and forcing a narrowing at every
// call site to reach a value is worse than the tuple it replaced. The methods
// carry the ergonomics, the way Result does in Rust.
const resultDeclarations = `export interface Result<T> {
  /** Whether the call succeeded. */
  readonly ok: boolean;
  /** The value, when the call succeeded. */
  readonly value?: T;
  /** The error, when the call failed. */
  readonly error?: Error;
  /** Returns the value, throwing the error if the call failed. */
  unwrap(): T;
  /** Returns the value, or the fallback if the call failed. */
  unwrapOr(fallback: T): T;
}
`
