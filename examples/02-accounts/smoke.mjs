import assert from "node:assert/strict";

import { boot } from "../harness.mjs";

const { account } = await boot(import.meta.dirname);

const alice = account.Open("Alice", 100);

// Fields read through to the Go value.
assert.equal(alice.Owner, "Alice");
assert.equal(alice.Balance, 100);
assert.deepEqual(alice.History, ["opened 100"]);

// A fallible call stays synchronous and carries its failure.
assert.equal(alice.Deposit(50).ok, true);
assert.equal(alice.Balance, 150);

const failed = alice.Withdraw(1000);
assert.equal(failed.ok, false);
assert.match(failed.error.message, /insufficient funds/);
assert.throws(() => failed.unwrap(), /insufficient funds/);

// Writes reach the Go value, so a method that reads the field sees them.
alice.Owner = "Alice B.";
assert.match(alice.Statement(), /^Alice B\.: 150/);

// A wrapper handed back resolves to the value it came from, so both sides move.
const bob = account.Open("Bob", 0);
assert.equal(account.Transfer(alice, bob, 50).ok, true);
assert.equal(alice.Balance, 100);
assert.equal(bob.Balance, 50);

// A plain object literal is accepted where a struct is taken by value.
assert.equal(account.Summarise({ Owner: "Carol", Balance: 7, History: [] }), "Carol holds 7");

// A misspelled field is rejected rather than silently zeroed.
assert.throws(() => account.Summarise({ Onwer: "Carol", Balance: 7, History: [] }));

// Result, both ways.
assert.equal(account.ParseAmount("12").unwrap(), 12);
assert.equal(account.ParseAmount("twelve").unwrapOr(0), 0);

// A plain snapshot is ordinary data: no handle, no methods, survives JSON.
const snapshot = alice.Snapshot();

assert.equal(snapshot.Owner, "Alice B.");
assert.equal(snapshot.Balance, 100);
assert.equal(typeof snapshot.release, "undefined");
assert.deepEqual(JSON.parse(JSON.stringify(snapshot)).Entries.length, 3);

// It is a snapshot, so later changes to the account do not reach it.
alice.Deposit(1);
assert.equal(snapshot.Balance, 100);

// Ignored in the manifest, so it never reached JavaScript.
assert.equal(typeof alice.Audit, "undefined");

// Releasing frees the handle; passing the wrapper back then fails loudly.
bob.release();
assert.throws(() => account.Transfer(alice, bob, 1), /released/);

console.log("02-accounts ok");

process.exit(0);
