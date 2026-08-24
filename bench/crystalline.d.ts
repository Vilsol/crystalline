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
export declare namespace payload {
  interface Point {
    X: number;
    Y: number;
    Label: string;
    Norm(): number;
    Shift(dx: number, dy: number): void;
  }
  function AddInts(a: number, b: number): number;
  function CountKeys(index: Record<string, number> | undefined): number;
  function Drain(values: AsyncIterable<number>): Promise<number>;
  function EchoBytes(data: Uint8Array | undefined): (Uint8Array | undefined);
  function EchoString(text: string): string;
  function MakeInts(n: number): (Array<number> | undefined);
  function MakeMap(n: number): (Record<string, number> | undefined);
  function MakePoints(n: number): (Array<payload.Point> | undefined);
  function MayFail(ok: boolean): Result<number>;
  function NewPoint(label: string): (payload.Point | undefined);
  function Noop(): void;
  function Rounds(n: number): Promise<number>;
  function Stream(n: number): AsyncIterable<number>;
  function SumFloats(values: Array<number> | undefined): number;
  function TakePoint(p: payload.Point): number;
}
export const initializeCrystalline: () => void;