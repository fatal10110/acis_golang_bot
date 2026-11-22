import type { ReactNode } from "react";
import { time } from "../lib/format";
import type { Crafter } from "../lib/types";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-2 py-1 text-sm">
      <dt className="text-slate-500">{label}</dt>
      <dd className="col-span-2 break-words">{children}</dd>
    </div>
  );
}

/** cycleOf is where the crafter's cycle stands, with its plan's chains or its next check. */
export function cycleOf(c: Crafter): string {
  if (c.phase !== "idle") return c.chains > 0 ? `${c.phase}, ${c.chains_done} of ${c.chains} chains` : c.phase;
  const next = time(c.next_check);
  return next ? `idle, next check ${next}` : "idle";
}

/** CrafterStatus is the rows of a crafter's status inside a bot's details list. */
export function CrafterStatus({ crafter: c }: { crafter: Crafter }) {
  const breaker = time(c.breaker_until);
  return (
    <>
      <Row label="Create Item">{c.create_item > 0 ? `level ${c.create_item}` : "not learned"}</Row>
      <Row label="Recipes">{c.book_known ? c.recipes : `${c.recipes} (book not read yet)`}</Row>
      <Row label="Craft orders">{`${c.orders} open, ${c.claimed} claimed`}</Row>
      <Row label="Craft cycle">{cycleOf(c)}</Row>
      <Row label="Last cycle">{c.last_cycle ? `${c.last_cycle} ${time(c.last_cycle_at)}` : "none yet"}</Row>
      <Row label="Breaker">{breaker ? `tripped, holds until ${breaker}` : "ok"}</Row>
    </>
  );
}
