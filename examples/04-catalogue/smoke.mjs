import assert from "node:assert/strict";

import { boot } from "../harness.mjs";

const { catalogue } = await boot(import.meta.dirname);

const items = catalogue.All();

assert.equal(items.length, 3);

// An enum: the constants are nameable, and the field carries one of them.
assert.equal(items[0].Status, catalogue.Status.StatusPublished);
assert.equal(catalogue.Status.StatusArchived, 2);

// A mapped type: Money crosses as the text that means it, never as cents.
assert.equal(items[0].Price, "49.99");

// A time crosses as a Date, through the embedded value that holds it.
assert.ok(items[0].Audited.CreatedAt instanceof Date);
assert.equal(items[0].Audited.CreatedAt.getUTCFullYear(), 2026);

// A method promoted from the embedded value is callable, as it is in Go.
assert.equal(items[0].Listed(), "1 Mar 2026");

// Writing a mapped field goes through the same conversion, and refuses what it
// cannot read.
items[1].Price = "20.00";
assert.equal(catalogue.Describe(items[1]), "Rope (draft) at 20.00");
assert.throws(() => {
	items[1].Price = "nonsense";
}, /12\.34/);

// Writing an enum field works the same way.
items[1].Status = catalogue.Status.StatusPublished;
assert.match(catalogue.Describe(items[1]), /published/);

// The other direction: Go asked for a Notifier, JavaScript supplies one.
const messages = [];
const touched = await catalogue.Restock({ Notify: (m) => messages.push(m) }, ["Lantern"]);

assert.equal(touched, 1);
assert.deepEqual(messages, ["restocked Lantern"]);

// An object without the method is refused rather than failing later.
await assert.rejects(catalogue.Restock({}, ["Anvil"]), /Notify/);

console.log("04-catalogue ok");

process.exit(0);
