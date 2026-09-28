/// <reference lib="es2018" />
/// <reference lib="dom" />
/// <reference lib="esnext.disposable" />
export type Result<T> = (
  | {
      /** Whether the call succeeded. */
      readonly ok: true;
      /** The value the call produced. */
      readonly value: T;
    }
  | {
      /** Whether the call succeeded. */
      readonly ok: false;
      /** Why the call failed. */
      readonly error: Error;
    }
) & {
  /** Returns the value, throwing the error if the call failed. */
  unwrap(): T;
  /** Returns the value, or the fallback if the call failed. */
  unwrapOr(fallback: T): T;
};
export declare namespace payload {
  interface Anchor {
    readonly X: number;
    readonly Y: number;
  }
  interface Line {
    Label: string;
    Values?: Array<number>;
    Sum(): number;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Point {
    X: number;
    Y: number;
    Label: string;
    Origin: payload.Anchor;
    Norm(): number;
    Shift(dx: number, dy: number): void;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Reading {
    readonly X: number;
    readonly Y: number;
    readonly Label: string;
    readonly Origin: payload.Anchor;
  }
  interface Record {
    ID: number;
    Name: string;
    Score: number;
    Active: boolean;
    Tags?: Array<string>;
    Lines?: Array<payload.Line>;
    Grid?: Array<Array<number> | undefined>;
    Totals?: Record<string, number>;
    Main: payload.Line;
    Parent?: payload.Line;
    Describe(): string;
    Total(): number;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface LinePlain {
    readonly Label: string;
    readonly Values?: Array<number>;
  }
  interface RecordPlain {
    readonly ID: number;
    readonly Name: string;
    readonly Score: number;
    readonly Active: boolean;
    readonly Tags?: Array<string>;
    readonly Lines?: Array<payload.LinePlain>;
    readonly Grid?: Array<Array<number> | undefined>;
    readonly Totals?: Record<string, number>;
    readonly Main: payload.LinePlain;
    readonly Parent?: payload.LinePlain;
  }
  function AddInts(a: number, b: number): number;
  function CountKeys(index: Record<string, number> | undefined): number;
  function Drain(values: AsyncIterable<number>): Promise<number>;
  function EchoBytes(data: Uint8Array | undefined): (Uint8Array | undefined);
  function EchoString(text: string): string;
  function MakeInts(n: number): (Array<number> | undefined);
  function MakeMap(n: number): (Record<string, number> | undefined);
  function MakePoints(n: number): (Array<payload.Point> | undefined);
  function MakeReadings(n: number): (Array<payload.Reading> | undefined);
  function MakeRecord(): (payload.Record | undefined);
  function MayFail(ok: boolean): Result<number>;
  function NewPoint(label: string): (payload.Point | undefined);
  function Noop(): void;
  function RecordData(): (payload.RecordPlain | undefined);
  function Rounds(n: number): Promise<number>;
  function Stream(n: number): AsyncIterable<number>;
  function SumFloats(values: Array<number> | undefined): number;
  function TakePoint(p: payload.Point): number;
}
export function boot(wasm: string | URL | BufferSource): Promise<{ payload: typeof payload }>;
export const initializeCrystalline: () => void;