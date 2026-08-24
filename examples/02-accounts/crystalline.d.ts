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
export declare namespace account {
  interface Account {
    Owner: string;
    Balance: number;
    History: Array<string>;
    Deposit(amount: number): Result<void>;
    Snapshot(): account.Statement;
    Statement(): string;
    Withdraw(amount: number): Result<void>;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Statement {
    readonly Owner: string;
    readonly Balance: number;
    readonly Entries?: Array<string>;
  }
  function Open(owner: string, opening: number): (account.Account | undefined);
  function ParseAmount(text: string): Result<number>;
  function Summarise(a: account.Account): string;
  function Transfer(from: account.Account | undefined, to: account.Account | undefined, amount: number): Result<void>;
}
export function boot(wasm: string | URL | BufferSource): Promise<{ account: typeof account }>;
export const initializeCrystalline: () => void;