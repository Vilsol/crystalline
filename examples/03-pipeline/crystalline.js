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

export let feed;

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
};