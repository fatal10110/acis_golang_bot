import type { ReactNode } from "react";
import { time } from "../lib/format";
import type { InspectEffect, Inspection, Penalties } from "../lib/types";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-2 py-1 text-sm">
      <dt className="text-slate-500">{label}</dt>
      <dd className="col-span-2 break-words">{children}</dd>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-1">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  );
}

/** leftOf is an effect's remaining time: "1h 5m", "2m 30s", "45s", or "no end". */
export function leftOf(e: InspectEffect): string {
  if (e.no_end) return "no end";
  const s = Math.max(0, e.left_s);
  if (s >= 3600) return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
  if (s >= 60) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${s}s`;
}

/** gradePenaltyOf is the grade penalty: what is over the Expertise grade, or "none". */
export function gradePenaltyOf(p: Penalties): string {
  if (!p.grade) return "none";
  const parts = [];
  if (p.grade_weapon) parts.push("weapon");
  if (p.grade_armor > 0) parts.push(`${p.grade_armor} armor ${p.grade_armor === 1 ? "piece" : "pieces"}`);
  return parts.length > 0 ? `on: ${parts.join(", ")}` : "on";
}

/** runSpeedOf is the run speed against the class's base one, when known. */
export function runSpeedOf(p: Penalties): string {
  return p.base_run_speed ? `${p.run_speed} of base ${p.base_run_speed}` : String(p.run_speed);
}

function named(name: string, enchant?: number): string {
  return enchant ? `+${enchant} ${name}` : name;
}

/** InspectView is a bot's inventory, worn gear, penalties and effects as its inspect answered them. */
export function InspectView({ inspection: v }: { inspection: Inspection }) {
  if (v.state === "offline") return <p className="text-sm text-slate-500">The bot is offline: no live data to show.</p>;
  if (v.state === "unresponsive") return <p className="text-sm text-amber-700">The bot did not answer the inspect.</p>;
  const inv = v.inventory;
  const p = v.penalties;
  return (
    <div className="space-y-4">
      <p className="text-xs text-slate-500">
        {v.state === "stale" ? "The server sent no inventory in time: shown as the bot last knew it, " : "Taken "}
        {time(v.at)}
      </p>
      {inv && (
        <Section title="Inventory">
          <p className="text-sm">
            Weight {inv.load} / {inv.max_load} ({inv.percent}%)
          </p>
          {inv.items.length === 0 ? (
            <p className="text-sm text-slate-500">Nothing carried.</p>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-slate-500">
                  <th className="font-normal">Item</th>
                  <th className="text-right font-normal">Count</th>
                  <th className="text-right font-normal">Weight</th>
                </tr>
              </thead>
              <tbody>
                {inv.items.map((it, i) => (
                  <tr key={i}>
                    <td>{named(it.name, it.enchant)}</td>
                    <td className="text-right tabular-nums">{it.count}</td>
                    <td className="text-right tabular-nums">{it.weight ?? "?"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Section>
      )}
      {v.equipment && (
        <Section title="Equipment">
          <dl>
            {v.equipment.length === 0 ? (
              <p className="text-sm text-slate-500">Nothing worn.</p>
            ) : (
              v.equipment.map((it, i) => (
                <Row key={i} label={it.slot}>
                  {named(it.name, it.enchant)}
                  {it.grade && ` (${it.grade} grade)`}
                </Row>
              ))
            )}
          </dl>
        </Section>
      )}
      {p && (
        <Section title="Penalties">
          <dl>
            <Row label="Weight penalty">{p.weight_level > 0 ? `level ${p.weight_level}` : "none"}</Row>
            <Row label="Grade penalty">{gradePenaltyOf(p)}</Row>
            <Row label="Death penalty">{p.death > 0 ? `level ${p.death}` : "none"}</Row>
            <Row label="Run speed">{runSpeedOf(p)}</Row>
          </dl>
        </Section>
      )}
      {v.effects && (
        <Section title="Buffs and debuffs">
          <dl>
            {v.effects.length === 0 ? (
              <p className="text-sm text-slate-500">No effects.</p>
            ) : (
              v.effects.map((e) => (
                <Row key={e.skill_id} label={`${e.name} ${e.level}`}>
                  {leftOf(e)}
                </Row>
              ))
            )}
          </dl>
        </Section>
      )}
    </div>
  );
}
