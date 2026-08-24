// Boots an example's wasm binary under node.
//
// The generated module knows how to start itself, so this only has to supply
// the Go runtime shim, which is a classic script rather than a module and so
// cannot be imported.

import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

export async function boot(dir) {
	const require = createRequire(pathToFileURL(join(dir, "harness.cjs")));

	// wasm_exec.js is a plain script that defines globalThis.Go.
	require(join(dir, "wasm_exec.js"));

	const module = await import(pathToFileURL(join(dir, "crystalline.js")));

	// Bytes rather than a URL: fetch cannot read a file URL under node.
	return module.boot(await readFile(join(dir, "app.wasm")));
}
