import type { ReactNode } from "react";
import { Button } from "./Button";

export function Loading({ what }: { what: string }) {
  return (
    <p role="status" className="animate-pulse py-6 text-center text-sm text-slate-500">
      Loading {what}…
    </p>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="py-10 text-center text-sm text-slate-500">{children}</div>;
}

export function Failed({ error, retry }: { error: unknown; retry?: () => void }) {
  return (
    <div role="alert" className="flex items-center gap-3 rounded-md bg-rose-50 p-3 text-sm text-rose-800 dark:bg-rose-950 dark:text-rose-200">
      <span>{error instanceof Error ? error.message : String(error)}</span>
      {retry && (
        <Button size="sm" onClick={retry}>
          Retry
        </Button>
      )}
    </div>
  );
}
