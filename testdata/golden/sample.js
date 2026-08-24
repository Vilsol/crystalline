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

export let sample;

export const initializeCrystalline = () => {
  if (globalThis['go']?.['app'] === undefined) {
    throw new Error('crystalline: globalThis.go.app is not set. Start the Go wasm module before calling initializeCrystalline().');
  }

  sample = {
    Basic: wrap(globalThis['go']['app']['sample']['Basic']),
    Cancellable: wrap(globalThis['go']['app']['sample']['Cancellable']),
    First: wrap(globalThis['go']['app']['sample']['First']),
    FooBar: wrap(globalThis['go']['app']['sample']['FooBar']),
    Keys: wrap(globalThis['go']['app']['sample']['Keys']),
    MayFail: wrap(globalThis['go']['app']['sample']['MayFail']),
    Middle: wrap(globalThis['go']['app']['sample']['Middle']),
    NewTicker: wrap(globalThis['go']['app']['sample']['NewTicker']),
    OnlyFails: wrap(globalThis['go']['app']['sample']['OnlyFails']),
    Rich: wrap(globalThis['go']['app']['sample']['Rich']),
    Stream: wrap(globalThis['go']['app']['sample']['Stream']),
    Sum: wrap(globalThis['go']['app']['sample']['Sum']),
    Ticks: wrap(globalThis['go']['app']['sample']['Ticks'])
  };
};