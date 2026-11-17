import { cn } from "../../lib/cn";
import { percent } from "../../lib/format";

const colors = { hp: "bg-rose-500", mp: "bg-sky-500", cp: "bg-amber-500" } as const;

/** Bar is a labelled HP, MP or CP gauge. */
export function Bar({ kind, value, max, className }: { kind: keyof typeof colors; value: number; max: number; className?: string }) {
  const p = percent(value, max);
  return (
    <div
      role="meter"
      aria-label={kind.toUpperCase()}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      aria-valuetext={`${value} / ${max}`}
      title={`${kind.toUpperCase()} ${value} / ${max}`}
      className={cn("h-1.5 w-20 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800", className)}
    >
      <div className={cn("h-full", colors[kind])} style={{ width: `${p}%` }} />
    </div>
  );
}
