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

export let account = pending('account');

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

  initialized = true;
};