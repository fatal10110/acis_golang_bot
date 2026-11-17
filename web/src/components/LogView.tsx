import { useEffect, useRef } from "react";
import { cn } from "../lib/cn";
import type { LogLine } from "../lib/types";

const levelColor: Record<string, string> = {
  ERROR: "text-rose-500",
  WARN: "text-amber-500",
  DEBUG: "text-slate-500",
};

/** LogView shows log lines, kept scrolled to the newest unless follow is off. */
export function LogView({ lines, follow = true, className }: { lines: readonly LogLine[]; follow?: boolean; className?: string }) {
  const ref = useRef<HTMLPreElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (follow && el) el.scrollTop = el.scrollHeight;
  }, [lines, follow]);
  return (
    <pre
      ref={ref}
      tabIndex={0}
      aria-label="Log lines"
      className={cn(
        "h-80 overflow-auto rounded-md bg-slate-950 p-3 font-mono text-xs leading-5 text-slate-200",
        className,
      )}
    >
      {lines.length === 0 ? (
        <span className="text-slate-500">No log lines.</span>
      ) : (
        lines.map((l, i) => (
          <div key={`${l.at}-${i}`} className={levelColor[l.level]}>
            {l.text}
          </div>
        ))
      )}
    </pre>
  );
}
