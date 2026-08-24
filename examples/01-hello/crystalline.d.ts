export declare namespace greeting {
  function Add(a: number, b: number): number;
  function Greet(name: string): string;
}
export function boot(wasm: string | URL | BufferSource): Promise<{ greeting: typeof greeting }>;
export const initializeCrystalline: () => void;