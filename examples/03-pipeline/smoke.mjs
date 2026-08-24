import assert from "node:assert/strict";

import { boot } from "../harness.mjs";

const { feed } = await boot(import.meta.dirname);

// A channel arrives as an async iterable. The call is a promise because the
// context has to be wired up before anything can be produced.
const running = new AbortController();
const primes = [];

for await (const prime of await feed.Primes(running.signal, 20)) {
	primes.push(prime);
}

assert.deepEqual(primes, [2, 3, 5, 7, 11, 13, 17, 19]);

// Aborting ends the stream where it stands.
const stopping = new AbortController();
const partial = [];

for await (const prime of await feed.Primes(stopping.signal, 1000)) {
	partial.push(prime);

	if (partial.length === 3) {
		stopping.abort();
	}
}

assert.deepEqual(partial, [2, 3, 5]);

// A channel parameter takes anything iterable.
assert.equal(await feed.Average([1, 2, 3, 4]), 2.5);

async function* generated() {
	yield 10;
	yield 20;
}

assert.equal(await feed.Average(generated()), 15);

// Slow and fallible: a promise carrying a Result.
assert.equal((await feed.Crunch(new AbortController().signal, 3)).unwrap(), 5);
assert.match((await feed.Crunch(new AbortController().signal, 0)).error.message, /must be positive/);

const cancelling = new AbortController();
const crunching = feed.Crunch(cancelling.signal, 1000);

cancelling.abort();

assert.match((await crunching).error.message, /cancelled/);

// Asynchronous by request rather than by necessity.
const digesting = feed.Digest("crystalline", 1000);

assert.equal(typeof digesting.then, "function");
assert.equal(typeof (await digesting), "string");

console.log("03-pipeline ok");

process.exit(0);
