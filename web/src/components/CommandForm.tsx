import { useState, type SyntheticEvent } from "react";
import { api } from "../lib/api";
import type { Spot } from "../lib/types";
import { useRun } from "../lib/useRun";
import { SpotPicker } from "./SpotPicker";
import { Button } from "./ui/Button";
import { Input } from "./ui/Field";

/**
 * CommandForm sends the panel's orders (docs/plan.md section 8): stay,
 * leave and logout at once; go with a spot; follow and assist with a
 * player's name. The answer is toasted: done, queued, or why the bot
 * refused.
 */
export function CommandForm({ account, level, spots }: { account: string; level: number; spots: readonly Spot[] | undefined }) {
  const { run, busy } = useRun();
  const [spot, setSpot] = useState("");
  const [player, setPlayer] = useState("");
  const send = (command: string) => run(`${account}: ${command}`, () => api.command(account, command));
  const onGo = (e: SyntheticEvent) => {
    e.preventDefault();
    if (spot) void send(`go ${spot}`);
  };
  const onPlayer = (verb: "follow" | "assist") => (e: SyntheticEvent) => {
    e.preventDefault();
    const name = player.trim();
    if (name) void send(`${verb} ${name}`);
  };
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        {(["stay", "leave", "logout"] as const).map((c) => (
          <Button key={c} disabled={busy} onClick={() => void send(c)}>
            {c}
          </Button>
        ))}
      </div>
      <form className="flex flex-col gap-2 sm:flex-row" onSubmit={onGo} aria-label="Go to a spot">
        {spots && spots.length > 0 ? (
          <SpotPicker spots={spots} level={level} value={spot} onChange={setSpot} />
        ) : (
          <p className="flex-1 text-sm text-slate-500">{spots ? "No farming spots: start the panel with -datapack." : "Loading spots…"}</p>
        )}
        <Button type="submit" variant="primary" disabled={busy || !spot}>
          go
        </Button>
      </form>
      <form className="flex flex-col gap-2 sm:flex-row" onSubmit={onPlayer("follow")} aria-label="Follow or assist a player">
        <Input
          aria-label="Player name"
          placeholder="Player name"
          value={player}
          autoComplete="off"
          onChange={(e) => {
            setPlayer(e.target.value);
          }}
          className="sm:max-w-60"
        />
        <div className="flex gap-2">
          <Button type="submit" disabled={busy || !player.trim()}>
            follow
          </Button>
          <Button disabled={busy || !player.trim()} onClick={(e) => { onPlayer("assist")(e); }}>
            assist
          </Button>
        </div>
      </form>
    </div>
  );
}
