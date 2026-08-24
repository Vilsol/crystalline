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

export let greeting = pending('greeting');

export const initializeCrystalline = () => {
  if (globalThis['go']?.['hello'] === undefined) {
    throw new Error('crystalline: globalThis.go.hello is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  greeting = {
    Add: wrap('greeting.Add', globalThis['go']['hello']['greeting']['Add']),
    Greet: wrap('greeting.Greet', globalThis['go']['hello']['greeting']['Greet'])
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

  return { greeting };
};