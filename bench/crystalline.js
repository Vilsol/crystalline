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

export let payload = pending('payload');

export const initializeCrystalline = () => {
  if (globalThis['go']?.['bench'] === undefined) {
    throw new Error('crystalline: globalThis.go.bench is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  payload = {
    AddInts: wrap(globalThis['go']['bench']['payload']['AddInts']),
    CountKeys: wrap(globalThis['go']['bench']['payload']['CountKeys']),
    Drain: wrap(globalThis['go']['bench']['payload']['Drain']),
    EchoBytes: wrap(globalThis['go']['bench']['payload']['EchoBytes']),
    EchoString: wrap(globalThis['go']['bench']['payload']['EchoString']),
    MakeInts: wrap(globalThis['go']['bench']['payload']['MakeInts']),
    MakeMap: wrap(globalThis['go']['bench']['payload']['MakeMap']),
    MakePoints: wrap(globalThis['go']['bench']['payload']['MakePoints']),
    MakeReadings: wrap(globalThis['go']['bench']['payload']['MakeReadings']),
    MayFail: wrap(globalThis['go']['bench']['payload']['MayFail']),
    NewPoint: wrap(globalThis['go']['bench']['payload']['NewPoint']),
    Noop: wrap(globalThis['go']['bench']['payload']['Noop']),
    Rounds: wrap(globalThis['go']['bench']['payload']['Rounds']),
    Stream: wrap(globalThis['go']['bench']['payload']['Stream']),
    SumFloats: wrap(globalThis['go']['bench']['payload']['SumFloats']),
    TakePoint: wrap(globalThis['go']['bench']['payload']['TakePoint'])
  };

  initialized = true;
};