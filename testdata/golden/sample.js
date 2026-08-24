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

export let generic = pending('generic');
export let sample = pending('sample');

export const initializeCrystalline = () => {
  if (globalThis['go']?.['app'] === undefined) {
    throw new Error('crystalline: globalThis.go.app is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  generic = {
    Numbers: wrap(globalThis['go']['app']['generic']['Numbers']),
    Strings: wrap(globalThis['go']['app']['generic']['Strings'])
  };
  sample = {
    Basic: wrap(globalThis['go']['app']['sample']['Basic']),
    Big: wrap(globalThis['go']['app']['sample']['Big']),
    Cancellable: wrap(globalThis['go']['app']['sample']['Cancellable']),
    First: wrap(globalThis['go']['app']['sample']['First']),
    FooBar: wrap(globalThis['go']['app']['sample']['FooBar']),
    Keys: wrap(globalThis['go']['app']['sample']['Keys']),
    MayFail: wrap(globalThis['go']['app']['sample']['MayFail']),
    Middle: wrap(globalThis['go']['app']['sample']['Middle']),
    NewTicker: wrap(globalThis['go']['app']['sample']['NewTicker']),
    OnlyFails: wrap(globalThis['go']['app']['sample']['OnlyFails']),
    Readings: wrap(globalThis['go']['app']['sample']['Readings']),
    Rich: wrap(globalThis['go']['app']['sample']['Rich']),
    Stream: wrap(globalThis['go']['app']['sample']['Stream']),
    Streamable: wrap(globalThis['go']['app']['sample']['Streamable']),
    Sum: wrap(globalThis['go']['app']['sample']['Sum']),
    Ticks: wrap(globalThis['go']['app']['sample']['Ticks']),
    Titles: globalThis['go']['app']['sample']['Titles'],
    Total: wrap(globalThis['go']['app']['sample']['Total'])
  };

  initialized = true;
};