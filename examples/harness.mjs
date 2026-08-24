// Boots an example's wasm binary under node, performing the same two steps
// index.html does: start the Go module, then initialise the generated bindings.
//
// It exists so each example can be checked by running it, rather than only by
// looking at it in a browser.

import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

export async function boot(dir) {
	const require = createRequire(pathToFileURL(join(dir, "harness.cjs")));

	// wasm_exec.js is a plain script that defines globalThis.Go.
	require(join(dir, "wasm_exec.js"));

	const go = new globalThis.Go();
	const { instance } = await WebAssembly.instantiate(await readFile(join(dir, "app.wasm")), go.importObject);

	// Not awaited: the example's main parks, so this promise never settles.
	go.run(instance);

	const module = await import(pathToFileURL(join(dir, "crystalline.js")));

	module.initializeCrystalline();

	return module;
}
