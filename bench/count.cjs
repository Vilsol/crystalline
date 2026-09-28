// Preloaded into node before wasm_exec_node.js, so that every crossing of the
// wasm bridge is counted: each import the Go runtime calls (wasm to JS), each
// js.FuncOf it makes, each call into one of those (JS to wasm), and each eval.
//
// wasm_exec.js assigns globalThis.Go as a class. Catching that assignment is
// the one place the import object can be wrapped before it is instantiated.
'use strict';

const counters = { crossings: 0, funcOf: 0, entered: 0, evals: 0 };
globalThis.crystallineCounters = counters;

// Go reaches eval as a property of the global object, so replacing it counts
// every compile. The indirect call keeps it a global eval.
const evaluate = globalThis.eval;
globalThis.eval = (source) => {
  counters.evals++;
  return (0, evaluate)(source);
};

let wrapped;

Object.defineProperty(globalThis, 'Go', {
  configurable: true,
  get() {
    return wrapped;
  },
  set(Base) {
    wrapped = class extends Base {
      constructor() {
        super();

        const imports = this.importObject.gojs;
        for (const [name, fn] of Object.entries(imports)) {
          if (!name.startsWith('syscall/js.') || typeof fn !== 'function') {
            continue;
          }

          imports[name] = (sp) => {
            counters.crossings++;
            return fn(sp);
          };
        }

        const make = this._makeFuncWrapper.bind(this);
        this._makeFuncWrapper = (id) => {
          counters.funcOf++;
          const inner = make(id);
          return function () {
            counters.entered++;
            return inner.apply(this, arguments);
          };
        };
      }
    };
  },
});
