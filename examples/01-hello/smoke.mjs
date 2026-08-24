import assert from "node:assert/strict";

import { boot } from "../harness.mjs";

const { greeting } = await boot(import.meta.dirname);

assert.equal(greeting.Greet("Vilsol"), "Hello, Vilsol!");
assert.equal(greeting.Greet("   "), "Hello, world!");
assert.equal(greeting.Add(2, 3), 5);

console.log("01-hello ok");

process.exit(0);
