import assert from "node:assert/strict";

import { boot } from "../harness.mjs";

// node has no localStorage, so the example's import is given one here. The
// browser supplies the real thing, and Go cannot tell the difference: the
// interface is the whole contract.
const kept = {};

// The methods are getItem and setItem rather than GetItem and SetItem because
// this example is generated with -case camel, and an import looks for the name
// its own surface uses. That is also why the flag is not optional here: the
// browser's own API is spelled the JavaScript way.
globalThis.localStorage = {
	getItem: (key) => kept[key] ?? "",
	setItem: (key, value) => { kept[key] = value; },
};

const { ledger } = await boot(import.meta.dirname);

// camelCase: the surface is spelled the way JavaScript usually is, and the Go
// spelling is gone rather than there as well.
assert.equal(typeof ledger.add, "function");
assert.equal(ledger.Add, undefined);

ledger.forget();
assert.equal(ledger.balance(), 0);

const first = ledger.add("opening", 250);

// ID lowers as a whole rather than becoming iD, and carries its value exactly:
// as a JavaScript number this would read back one lower.
assert.equal(typeof first.id, "bigint");
assert.equal(first.id, 9007199254740993n);

// An explicit name wins over the convention, which would have said "note".
assert.equal(first.description, "opening");
assert.equal(first.note, undefined);

assert.equal(ledger.balance(), 250);

// Go really reached the object this file supplied.
assert.equal(kept["crystalline.ledger.balance"], "250");

const second = ledger.add("coffee", -3);
assert.equal(second.id, 9007199254740994n);
assert.equal(ledger.balance(), 247);

first.release();
second.release();

console.log("05-ledger ok");

process.exit(0);
