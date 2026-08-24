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

export let greeting;

export const initializeCrystalline = () => {
  if (globalThis['go']?.['hello'] === undefined) {
    throw new Error('crystalline: globalThis.go.hello is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  greeting = {
    Add: wrap(globalThis['go']['hello']['greeting']['Add']),
    Greet: wrap(globalThis['go']['hello']['greeting']['Greet'])
  };
};