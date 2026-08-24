//go:build !js

package crystalline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
const jsWrapHelper = `const wrap = (name, fn) => {
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

// jsProfilingWrapHelper is the same wrapper, counting and timing what passes
// through it. The name is carried so a report can say which binding it was.
const jsProfilingWrapHelper = `const crystallineStats = new Map();

const wrap = (name, fn) => {
  return (...args) => {
    const started = performance.now();
    const result = fn.call(undefined, ...args);
    const elapsed = performance.now() - started;

    const seen = crystallineStats.get(name);
    if (seen === undefined) {
      crystallineStats.set(name, { name, calls: 1, ms: elapsed });
    } else {
      seen.calls += 1;
      seen.ms += elapsed;
    }

    if (globalThis.goInternalError) {
      const error = new Error(globalThis.goInternalError);
      globalThis.goInternalError = undefined;
      throw error;
    }
    return result;
  }
};

/** What each binding cost, heaviest first. */
export const stats = () => [...crystallineStats.values()].sort((a, b) => b.ms - a.ms);

/** Forgets everything counted so far. */
export const resetStats = () => crystallineStats.clear();`

// tsProfiling declares what profiling adds to the module.
const tsProfiling = `export function stats(): Array<{ name: string; calls: number; ms: number }>;
export function resetStats(): void;
`

// jsPendingHelper stands in for a namespace until initializeCrystalline runs.
//
// The bindings cannot exist before the wasm module does, and an undefined
// namespace fails somewhere else entirely: destructuring one snapshots the
// undefined, and the error surfaces later as a missing property on nothing.
// Reading through this says what actually went wrong.
//
// Which of two things went wrong depends on whether initialisation has since
// happened. Before it, the module was used too early. After it, the caller is
// holding a copy taken too early -- destructuring an export snapshots it, so a
// local keeps pointing at this proxy however many times the real binding is
// reassigned. Telling the second case to call initializeCrystalline() would be
// advice it has already followed.
//
// Symbols and then are let through, so that logging, awaiting and the probing
// bundlers do are not turned into spurious failures.
func jsPendingHelper(style jsStyle) string {
	return "let initialized = false;\n\n" +
		"const pending = (name) => new Proxy({}, {\n" +
		"  get(target, property) {\n" +
		"    if (typeof property === " + style.quoted("symbol") + " || property === " + style.quoted("then") + ") {\n" +
		"      return undefined;\n" +
		"    }\n" +
		"    if (initialized) {\n" +
		"      throw new Error(" + style.quoted("crystalline: this ") + " + name + " +
		style.quoted(" was captured before initializeCrystalline() ran, so it is a stale copy. Read it from the module instead of destructuring it earlier, or move the import after initialisation.") + ");\n" +
		"    }\n" +
		"    throw new Error(" + style.quoted("crystalline: ") + " + name + " + style.quoted(".") + " + String(property) + " +
		style.quoted(" was read before initializeCrystalline() ran. Start the Go wasm module, then call initializeCrystalline().") + ");\n" +
		"  }\n" +
		"});"
}

// jsLoader renders the loader the module exports.
//
// Starting a Go wasm module is six lines that every project wrote the same way
// and one of which is a trap: run() must not be awaited, because the program
// parks so that JavaScript can drive it, and awaiting it hangs forever. The
// namespaces are returned as well as exported, so a caller who destructures
// gets the bound ones rather than a copy taken too early.
func jsLoader(style jsStyle, namespaces []string) string {
	var out strings.Builder

	out.WriteString("export const boot = async (wasm) => {\n")
	out.WriteString("  if (globalThis[" + style.quoted("Go") + "] === undefined) {\n")
	out.WriteString("    throw new Error(" + style.quoted("crystalline: the Go runtime shim is missing. Load wasm_exec.js from your Go toolchain before calling boot().") + ");\n")
	out.WriteString("  }\n\n")
	out.WriteString("  const runtime = new globalThis[" + style.quoted("Go") + "]();\n\n")

	// Bytes as well as a URL: a bundler may inline the binary, and fetch cannot
	// read a file URL outside a browser.
	out.WriteString("  const source = wasm instanceof ArrayBuffer || ArrayBuffer.isView(wasm)\n")
	out.WriteString("    ? wasm\n")
	out.WriteString("    : await (await fetch(wasm)).arrayBuffer();\n\n")
	out.WriteString("  const { instance } = await WebAssembly.instantiate(source, runtime.importObject);\n\n")
	out.WriteString("  // Not awaited: the Go program parks, so this never settles.\n")
	out.WriteString("  runtime.run(instance);\n\n")
	out.WriteString("  initializeCrystalline();\n\n")
	out.WriteString("  return { " + strings.Join(namespaces, ", ") + " };\n")
	out.WriteString("};")

	return out.String()
}

// tsLoader declares the loader and what it hands back.
func tsLoader(namespaces []string) string {
	typed := make([]string, 0, len(namespaces))
	for _, name := range namespaces {
		typed = append(typed, name+": typeof "+name)
	}

	return "export function boot(wasm: string | URL | BufferSource): Promise<{ " + strings.Join(typed, "; ") + " }>;\n"
}

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

	// Warnings lists what was bound at a cost worth knowing about.
	Warnings []Warning
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
