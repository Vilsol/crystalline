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

export let catalogue = pending('catalogue');

export const initializeCrystalline = () => {
  if (globalThis['go']?.['catalogue'] === undefined) {
    throw new Error('crystalline: globalThis.go.catalogue is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  catalogue = {
    All: wrap('catalogue.All', globalThis['go']['catalogue']['catalogue']['All']),
    Describe: wrap('catalogue.Describe', globalThis['go']['catalogue']['catalogue']['Describe']),
    Restock: wrap('catalogue.Restock', globalThis['go']['catalogue']['catalogue']['Restock']),
    Status: globalThis['go']['catalogue']['catalogue']['Status']
  };

  initialized = true;
};

export const boot = async (wasm) => {
  if (globalThis['Go'] === undefined) {
    throw new Error('crystalline: the Go runtime shim is missing. Load wasm_exec.js from your Go toolchain before calling boot().');
  }

  const runtime = new globalThis['Go']();

  let source = wasm;

  if (!(wasm instanceof ArrayBuffer) && !ArrayBuffer.isView(wasm)) {
    const response = await fetch(wasm);

    if (!response.ok) {
      throw new Error('crystalline: could not fetch ' + wasm + ': ' + response.status);
    }

    source = await response.arrayBuffer();
  }

  const { instance } = await WebAssembly.instantiate(source, runtime.importObject);

  // Not awaited: the Go program parks, so this never settles.
  runtime.run(instance);

  initializeCrystalline();

  return { catalogue };
};