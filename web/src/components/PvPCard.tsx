import type { ReactNode } from "react";
import { api } from "../lib/api";
import type { Bot, PvPModes } from "../lib/types";
import { useRun } from "../lib/useRun";
import { Button } from "./ui/Button";
import { Card } from "./ui/Card";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-2 py-1 text-sm">
      <dt className="text-slate-500">{label}</dt>
      <dd className="col-span-2 break-words">{children}</dd>
    </div>
  );
}

const switches: { key: keyof PvPModes; label: string }[] = [
  { key: "duels", label: "Duels" },
  { key: "hunt", label: "Hunt" },
  { key: "arenas", label: "Arenas" },
  { key: "pk", label: "PK" },
];

/**
 * modesOf is the switches the bot plays: the panel's, else the ones its
 * last session reported (its preset's defaults), else all off before its
 * first session.
 */
export function modesOf(b: Bot): PvPModes {
  const m = b.pvp ?? b.live?.pvp;
  return { duels: m?.duels ?? false, hunt: m?.hunt ?? false, arenas: m?.arenas ?? false, pk: m?.pk ?? false };
}

/**
 * PvPCard is a bot's PvP modes (docs/pvp/modes.md "Panel and metrics
 * (X12)"): the four switches the operator flips, a reset to the preset's
 * defaults, fight back, the kill counts and the duels of this run.
 */
export function PvPCard({ bot: b }: { bot: Bot }) {
  const { run, busy } = useRun();
  const modes = modesOf(b);
  const p = b.live?.pvp;
  const flip = (key: keyof PvPModes) => {
    const next = { ...modes, [key]: !modes[key] };
    void run(`${b.account}: pvp ${key} ${next[key] ? "on" : "off"}`, () => api.setPvP(b.account, next));
  };
  return (
    <Card
      title={
        <span className="flex items-center justify-between">
          PvP
          <Button
            disabled={busy || !b.pvp}
            onClick={() => void run(`${b.account}: pvp defaults`, () => api.setPvP(b.account, null))}
          >
            Defaults
          </Button>
        </span>
      }
    >
      <div className="flex flex-wrap gap-4 pb-2 text-sm">
        {switches.map((s) => (
          <label key={s.key} className="flex items-center gap-1">
            <input
              type="checkbox"
              checked={modes[s.key]}
              disabled={busy}
              onChange={() => {
                flip(s.key);
              }}
            />
            {s.label}
          </label>
        ))}
      </div>
      <dl className="divide-y divide-slate-100 dark:divide-slate-800">
        <Row label="Switches">{b.pvp ? "set in the panel" : "preset defaults"}</Row>
        {p ? (
          <>
            <Row label="Fight back">{p.fight_back ? "on" : "off"}</Row>
            <Row label="PvP kills">{p.pvp_kills}</Row>
            <Row label="PK kills">{p.pk_kills}</Row>
            <Row label="Duels this run">{`${p.duels_won} won, ${p.duels_lost} lost, ${p.duels_tied} tied`}</Row>
            <Row label="Last duel">{p.last_duel ?? "none"}</Row>
          </>
        ) : (
          <Row label="Counts">shown after the bot's first session</Row>
        )}
      </dl>
    </Card>
  );
}
