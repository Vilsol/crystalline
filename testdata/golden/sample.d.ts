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
export declare namespace sample {
  interface FnSample {
    FirstValue: string;
    SecondValue: number;
    ThirdValue: number;
    A(x: boolean): boolean;
    B(x: boolean): boolean;
    C(x: boolean): Promise<boolean>;
    One(): string;
    Three(): number;
    Two(): number;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Reading {
    readonly Label: string;
    readonly Values?: Array<number>;
    readonly Peak: sample.Sample;
  }
  interface Richer {
    Blob?: Uint8Array;
    Lookup?: Record<string, number>;
    MightBeNil?: Array<string>;
    NeverNil: Array<string>;
    Pointed?: sample.FnSample;
    Inner: sample.FnSample;
    Apply(other: sample.Richer): number;
    Configure(s: sample.FnSample): string;
    Fails(): Result<void>;
    WithCallback(cb: (v: string) => Promise<number>): Promise<boolean>;
    WithText(cb: (v: string) => Promise<string>): Promise<string>;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Sample {
    readonly At: string;
    readonly Value: number;
  }
  interface Ticker {
    Name: string;
    readonly Events: AsyncIterable<string>;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  function Basic(): number;
  function Big(n: number): number;
  function Cancellable(signal: AbortSignal, label: string): Promise<Result<string>>;
  function First(values: AsyncIterable<number>): Promise<number>;
  function FooBar(): sample.FnSample;
  function Keys(ids: Array<number> | undefined, seed: number): number;
  function MayFail(ok: boolean): Result<string>;
  function Middle(first: sample.FnSample | undefined, label: string): string;
  function NewTicker(): sample.Ticker;
  function OnlyFails(ok: boolean): Result<void>;
  function Readings(count: number): (Array<sample.Reading> | undefined);
  function Rich(): sample.Richer;
  function Stream(count: number): AsyncIterable<string>;
  function Streamable(signal: AbortSignal, ok: boolean): Result<AsyncIterable<number>>;
  function Sum(values: AsyncIterable<number>): Promise<number>;
  function Ticks(signal: AbortSignal, count: number): AsyncIterable<number>;
  const Titles: Record<number, string> | undefined;
}
export const initializeCrystalline: () => void;