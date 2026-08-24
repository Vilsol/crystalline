import assert from "node:assert/strict";

import { boot } from "../harness.mjs";

// A namespace read before initialisation says so, rather than being undefined
// and failing somewhere else entirely.
const early = await import("./crystalline.js");

assert.throws(() => early.greeting.Greet, /was read before initializeCrystalline/);

// Destructuring an export snapshots it, so this local keeps pointing at the
// placeholder even after initialisation. The message has to say that, rather
// than repeating advice that has already been followed.
const { greeting: captured } = early;

const { greeting } = await boot(import.meta.dirname);

assert.throws(() => captured.Greet, /captured before initializeCrystalline/);

assert.equal(greeting.Greet("Vilsol"), "Hello, Vilsol!");
assert.equal(greeting.Greet("   "), "Hello, world!");
assert.equal(greeting.Add(2, 3), 5);

console.log("01-hello ok");

process.exit(0);
