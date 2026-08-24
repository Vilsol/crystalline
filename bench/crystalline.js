const wrap = (name, fn) => {
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
    AddInts: wrap('payload.AddInts', globalThis['go']['bench']['payload']['AddInts']),
    CountKeys: wrap('payload.CountKeys', globalThis['go']['bench']['payload']['CountKeys']),
    Drain: wrap('payload.Drain', globalThis['go']['bench']['payload']['Drain']),
    EchoBytes: wrap('payload.EchoBytes', globalThis['go']['bench']['payload']['EchoBytes']),
    EchoString: wrap('payload.EchoString', globalThis['go']['bench']['payload']['EchoString']),
    MakeInts: wrap('payload.MakeInts', globalThis['go']['bench']['payload']['MakeInts']),
    MakeMap: wrap('payload.MakeMap', globalThis['go']['bench']['payload']['MakeMap']),
    MakePoints: wrap('payload.MakePoints', globalThis['go']['bench']['payload']['MakePoints']),
    MakeReadings: wrap('payload.MakeReadings', globalThis['go']['bench']['payload']['MakeReadings']),
    MayFail: wrap('payload.MayFail', globalThis['go']['bench']['payload']['MayFail']),
    NewPoint: wrap('payload.NewPoint', globalThis['go']['bench']['payload']['NewPoint']),
    Noop: wrap('payload.Noop', globalThis['go']['bench']['payload']['Noop']),
    Rounds: wrap('payload.Rounds', globalThis['go']['bench']['payload']['Rounds']),
    Stream: wrap('payload.Stream', globalThis['go']['bench']['payload']['Stream']),
    SumFloats: wrap('payload.SumFloats', globalThis['go']['bench']['payload']['SumFloats']),
    TakePoint: wrap('payload.TakePoint', globalThis['go']['bench']['payload']['TakePoint'])
  };

  initialized = true;
};

export const boot = async (wasm) => {
  if (globalThis['Go'] === undefined) {
    throw new Error('crystalline: the Go runtime shim is missing. Load wasm_exec.js from your Go toolchain before calling boot().');
  }

  const runtime = new globalThis['Go']();

  const source = wasm instanceof ArrayBuffer || ArrayBuffer.isView(wasm)
    ? wasm
    : await (await fetch(wasm)).arrayBuffer();

  const { instance } = await WebAssembly.instantiate(source, runtime.importObject);

  // Not awaited: the Go program parks, so this never settles.
  runtime.run(instance);

  initializeCrystalline();

  return { payload };
};