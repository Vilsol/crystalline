export declare namespace catalogue {
  interface Audited {
    CreatedAt: Date;
    Listed(): string;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Item {
    Audited: catalogue.Audited;
    Name: string;
    Price: string;
    Status: catalogue.Status;
    Listed(): string;
    Reprice(to: string): void;
    /** Releases the Go resources behind this wrapper. */
    release(): void;
    [Symbol.dispose](): void;
  }
  interface Notifier {
    Notify(message: string): void;
  }
  type Status = 0 | 1 | 2;
  const Status: {
    readonly StatusDraft: 0;
    readonly StatusPublished: 1;
    readonly StatusArchived: 2;
  };
  function All(): (Array<catalogue.Item | undefined> | undefined);
  function Describe(i: catalogue.Item | undefined): string;
  function Restock(notify: catalogue.Notifier, names: Array<string> | undefined): Promise<number>;
}
export function boot(wasm: string | URL | BufferSource): Promise<{ catalogue: typeof catalogue }>;
export const initializeCrystalline: () => void;