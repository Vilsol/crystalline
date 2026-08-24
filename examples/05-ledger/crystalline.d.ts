/// <reference lib="es2018" />
/// <reference lib="dom" />
/// <reference lib="esnext.disposable" />
export declare namespace ledger {
  interface Entry {
    id: bigint;
    description: string;
    amount: number;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  function add(note: string, amount: number): ledger.Entry;
  function balance(): number;
  function forget(): void;
}
export function boot(wasm: string | URL | BufferSource): Promise<{ ledger: typeof ledger }>;
export const initializeCrystalline: () => void;