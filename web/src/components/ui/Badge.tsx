import type { ReactNode } from "react";
import { cn } from "../../lib/cn";
import type { Status } from "../../lib/format";

export type Tone = "green" | "amber" | "red" | "slate" | "sky";

const tones: Record<Tone, string> = {
  green: "bg-emerald-100 text-emerald-800 dark:bg-emerald-900/50 dark:text-emerald-300",
  amber: "bg-amber-100 text-amber-800 dark:bg-amber-900/50 dark:text-amber-300",
  red: "bg-rose-100 text-rose-800 dark:bg-rose-900/50 dark:text-rose-300",
  slate: "bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-slate-300",
  sky: "bg-sky-100 text-sky-800 dark:bg-sky-900/50 dark:text-sky-300",
};

export function Badge({ tone = "slate", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={cn("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap", tones[tone])}>
      {children}
    </span>
  );
}

const statusTones: Record<Status, Tone> = {
  running: "green",
  online: "sky",
  "cooling down": "amber",
  stopping: "slate",
  halted: "red",
  none: "slate",
  offline: "slate",
};

export function StatusBadge({ status }: { status: Status }) {
  return <Badge tone={statusTones[status]}>{status}</Badge>;
}
