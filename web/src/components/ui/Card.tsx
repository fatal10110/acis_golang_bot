import type { ReactNode } from "react";
import { cn } from "../../lib/cn";

export function Card({ title, children, className }: { title?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn("rounded-lg border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900", className)}>
      {title && <h2 className="mb-2 text-sm font-semibold text-slate-600 dark:text-slate-400">{title}</h2>}
      {children}
    </section>
  );
}
