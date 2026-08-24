const wrap = (fn) => {
  return (...args) => {
    const result = fn.call(undefined, ...args);
    if (globalThis.goInternalError) {
      const error = new Error(globalThis.goInternalError);
      globalThis.goInternalError = undefined;
      throw error;
    }
    return result;
  }
};

let initialized = false;

const pending = (name) => new Proxy({}, {
  get(target, property) {
    if (typeof property === 'symbol' || property === 'then') {
      return undefined;
    }
    if (initialized) {
      throw new Error('crystalline: this ' + name + ' was captured before initializeCrystalline() ran, so it is a stale copy. Read it from the module instead of destructuring it earlier, or move the import after initialisation.');
    }
    throw new Error('crystalline: ' + name + '.' + String(property) + ' was read before initializeCrystalline() ran. Start the Go wasm module, then call initializeCrystalline().');
  }
});

export let feed = pending('feed');

export const initializeCrystalline = () => {
  if (globalThis['go']?.['pipeline'] === undefined) {
    throw new Error('crystalline: globalThis.go.pipeline is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  feed = {
    Average: wrap(globalThis['go']['pipeline']['feed']['Average']),
    Crunch: wrap(globalThis['go']['pipeline']['feed']['Crunch']),
    Digest: wrap(globalThis['go']['pipeline']['feed']['Digest']),
    Primes: wrap(globalThis['go']['pipeline']['feed']['Primes'])
  };

  initialized = true;
};