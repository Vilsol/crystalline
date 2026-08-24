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

export let account;

export const initializeCrystalline = () => {
  if (globalThis['go']?.['accounts'] === undefined) {
    throw new Error('crystalline: globalThis.go.accounts is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  account = {
    Open: wrap(globalThis['go']['accounts']['account']['Open']),
    ParseAmount: wrap(globalThis['go']['accounts']['account']['ParseAmount']),
    Summarise: wrap(globalThis['go']['accounts']['account']['Summarise']),
    Transfer: wrap(globalThis['go']['accounts']['account']['Transfer'])
  };
};