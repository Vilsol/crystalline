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

export let generic = pending('generic');
export let marshal = pending('marshal');
export let sample = pending('sample');

export const initializeCrystalline = () => {
  if (globalThis['go']?.['app'] === undefined) {
    throw new Error('crystalline: globalThis.go.app is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  generic = {
    Numbers: wrap('generic.Numbers', globalThis['go']['app']['generic']['Numbers']),
    Strings: wrap('generic.Strings', globalThis['go']['app']['generic']['Strings'])
  };
  marshal = {
    Brighten: wrap('marshal.Brighten', globalThis['go']['app']['marshal']['Brighten'])
  };
  sample = {
    Advance: wrap('sample.Advance', globalThis['go']['app']['sample']['Advance']),
    Basic: wrap('sample.Basic', globalThis['go']['app']['sample']['Basic']),
    Big: wrap('sample.Big', globalThis['go']['app']['sample']['Big']),
    Cancellable: wrap('sample.Cancellable', globalThis['go']['app']['sample']['Cancellable']),
    First: wrap('sample.First', globalThis['go']['app']['sample']['First']),
    FooBar: wrap('sample.FooBar', globalThis['go']['app']['sample']['FooBar']),
    Keys: wrap('sample.Keys', globalThis['go']['app']['sample']['Keys']),
    MakeEmbedder: wrap('sample.MakeEmbedder', globalThis['go']['app']['sample']['MakeEmbedder']),
    MakeStamped: wrap('sample.MakeStamped', globalThis['go']['app']['sample']['MakeStamped']),
    MayFail: wrap('sample.MayFail', globalThis['go']['app']['sample']['MayFail']),
    MaybeStamp: wrap('sample.MaybeStamp', globalThis['go']['app']['sample']['MaybeStamp']),
    Middle: wrap('sample.Middle', globalThis['go']['app']['sample']['Middle']),
    NewTicker: wrap('sample.NewTicker', globalThis['go']['app']['sample']['NewTicker']),
    OnlyFails: wrap('sample.OnlyFails', globalThis['go']['app']['sample']['OnlyFails']),
    Phase: globalThis['go']['app']['sample']['Phase'],
    Readings: wrap('sample.Readings', globalThis['go']['app']['sample']['Readings']),
    Replay: wrap('sample.Replay', globalThis['go']['app']['sample']['Replay']),
    Rich: wrap('sample.Rich', globalThis['go']['app']['sample']['Rich']),
    Stream: wrap('sample.Stream', globalThis['go']['app']['sample']['Stream']),
    Streamable: wrap('sample.Streamable', globalThis['go']['app']['sample']['Streamable']),
    Sum: wrap('sample.Sum', globalThis['go']['app']['sample']['Sum']),
    TakesTime: wrap('sample.TakesTime', globalThis['go']['app']['sample']['TakesTime']),
    Ticks: wrap('sample.Ticks', globalThis['go']['app']['sample']['Ticks']),
    Titles: globalThis['go']['app']['sample']['Titles'],
    Total: wrap('sample.Total', globalThis['go']['app']['sample']['Total'])
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

  return { generic, marshal, sample };
};