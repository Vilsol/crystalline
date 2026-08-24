export interface Result<T> {
  /** Whether the call succeeded. */
  readonly ok: boolean;
  /** The value, when the call succeeded. */
  readonly value?: T;
  /** The error, when the call failed. */
  readonly error?: Error;
  /** Returns the value, throwing the error if the call failed. */
  unwrap(): T;
  /** Returns the value, or the fallback if the call failed. */
  unwrapOr(fallback: T): T;
}
export declare namespace feed {
  function Average(values: AsyncIterable<number>): Promise<number>;
  function Crunch(signal: AbortSignal, rounds: number): Promise<Result<number>>;
  function Digest(text: string, rounds: number): Promise<string>;
  function Primes(signal: AbortSignal, limit: number): Promise<AsyncIterable<number>>;
}
export const initializeCrystalline: () => void;