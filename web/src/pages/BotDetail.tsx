import type { ReactNode } from "react";
import { CommandForm } from "../components/CommandForm";
import { CrafterStatus } from "../components/CrafterStatus";
import { InspectView } from "../components/InspectView";
import type { Confirm } from "../components/ConfirmDialog";
import { PresetOptions } from "../components/CreateDialog";
import { LogView } from "../components/LogView";
import { PvPCard } from "../components/PvPCard";
import { StatusBadge } from "../components/ui/Badge";
import { Bar } from "../components/ui/Bar";
import { Button } from "../components/ui/Button";
import { Card } from "../components/ui/Card";
import { Select } from "../components/ui/Field";
import { Empty, Failed, Loading } from "../components/ui/States";
import { api } from "../lib/api";
import { activityOf, characterOf, className, classOf, levelOf, partyOf, statusOf, time, whereOf } from "../lib/format";
import { useClassNames, useInspection, useLogs, usePresets, useSpots } from "../lib/queries";
import type { Bot } from "../lib/types";
import { useRun } from "../lib/useRun";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-2 py-1 text-sm">
      <dt className="text-slate-500">{label}</dt>
      <dd className="col-span-2 break-words">{children}</dd>
    </div>
  );
}

function Gauge({ kind, value, max }: { kind: "hp" | "mp" | "cp"; value: number; max: number }) {
  return (
    <div className="flex items-center gap-2">
      <Bar kind={kind} value={value} max={max} className="w-32" />
      <span className="tabular-nums">
        {value} / {max}
      </span>
    </div>
  );
}

/** InspectCard asks the bot for its inventory, gear, penalties and effects when the page opens, and again on Refresh. */
function InspectCard({ account }: { account: string }) {
  const q = useInspection(account);
  let body: ReactNode;
  if (q.isPending) body = <Loading what="the inspect" />;
  else if (q.isError) body = <Failed error={q.error} retry={() => void q.refetch()} />;
  else body = <InspectView inspection={q.data} />;
  return (
    <Card
      title={
        <span className="flex items-center justify-between">
          Inspect
          <Button disabled={q.isFetching} onClick={() => void q.refetch()}>
            Refresh
          </Button>
        </span>
      }
    >
      {body}
    </Card>
  );
}

export function BotDetail({ account, bots, ask }: { account: string; bots: Bot[] | undefined; ask: (c: Confirm) => void }) {
  const { names } = useClassNames();
  const presets = usePresets();
  const spots = useSpots();
  const logs = useLogs(account, 200);
  const { run, busy } = useRun();
  if (bots === undefined) return <Loading what="the bot" />;
  const b = bots.find((x) => x.account === account);
  if (!b) {
    return (
      <Empty>
        No bot {account}. <a className="text-sky-600 underline" href="#/bots">Back to the bots</a>
      </Empty>
    );
  }
  const l = b.live;
  const act = (action: "spawn" | "despawn" | "respawn") => void run(`${action} ${account}`, () => api.act(account, action));
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <a href="#/bots" className="text-sm text-sky-700 hover:underline dark:text-sky-400">
          ← Bots
        </a>
        <h1 className="text-xl font-semibold">{characterOf(b) || account}</h1>
        <StatusBadge status={statusOf(b)} />
        <div className="ml-auto flex flex-wrap gap-2">
          {b.online ? (
            <>
              <Button disabled={busy} onClick={() => { act("respawn"); }}>
                Respawn
              </Button>
              <Button
                disabled={busy}
                onClick={() => {
                  ask({ title: `Despawn ${account}?`, body: "The bot logs out and stays out until spawned again.", action: "Despawn", run: () => { act("despawn"); } });
                }}
              >
                Despawn
              </Button>
            </>
          ) : (
            <>
              <Button variant="primary" disabled={busy} onClick={() => { act("spawn"); }}>
                Spawn
              </Button>
              <Button
                variant="danger"
                disabled={busy}
                onClick={() => {
                  ask({
                    title: `Delete ${account}?`,
                    body: "The bot leaves the registry. Its account is never reused; its character stays on the server.",
                    action: "Delete",
                    danger: true,
                    run: () => {
                      void run(`delete ${account}`, () => api.remove(account)).then((ok) => {
                        if (ok) window.location.hash = "#/bots";
                      });
                    },
                  });
                }}
              >
                Delete
              </Button>
            </>
          )}
        </div>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Snapshot">
          <dl className="divide-y divide-slate-100 dark:divide-slate-800">
            <Row label="Account">{b.account}</Row>
            <Row label="Class">{className(names, classOf(b))}</Row>
            <Row label="Level">{levelOf(b) || ""}</Row>
            <Row label="Preset">
              <Select
                aria-label="Preset"
                value={b.preset}
                disabled={busy || !presets.data}
                onChange={(e) => {
                  const p = e.target.value;
                  void run(`${account}: preset ${p}`, () => api.setPreset(account, p));
                }}
                className="w-auto"
              >
                {presets.data ? <PresetOptions presets={presets.data.presets} /> : <option>{b.preset}</option>}
              </Select>
            </Row>
            <Row label="Party mode">
              <Select
                aria-label="Party mode"
                value={b.party_mode === "off" ? "off" : "auto"}
                disabled={busy}
                onChange={(e) => {
                  const m = e.target.value === "off" ? "off" : "auto";
                  void run(`${account}: party ${m}`, () => api.setPartyMode(account, m));
                }}
                className="w-auto"
              >
                <option value="auto">auto</option>
                <option value="off">off</option>
              </Select>
            </Row>
            {l && (
              <>
                <Row label="State">{l.state}</Row>
                <Row label="HP">
                  <Gauge kind="hp" value={l.hp} max={l.max_hp} />
                </Row>
                <Row label="MP">
                  <Gauge kind="mp" value={l.mp} max={l.max_mp} />
                </Row>
                <Row label="CP">
                  <Gauge kind="cp" value={l.cp} max={l.max_cp} />
                </Row>
                <Row label="Rule / order">{activityOf(b)}</Row>
                <Row label="Spot">{l.spot ?? "none"}</Row>
                <Row label="Walk">{l.walk ?? "ok"}</Row>
                <Row label="Party">{l.leader ? [l.leader, ...(l.members ?? [])].join(", ") : ""}</Row>
                <Row label="Party slot">{partyOf(b)}</Row>
                {l.crafter && <CrafterStatus crafter={l.crafter} />}
                <Row label="Reconnects">{l.reconnects}</Row>
                {l.err && <Row label="Last error">{l.err}</Row>}
                <Row label="Snapshot at">{time(l.at)}</Row>
              </>
            )}
            <Row label="Where">{whereOf(b)}</Row>
            {b.stop_reason && (
              <Row label="Stopped">
                {b.stop_reason} {time(b.stopped_at)}
              </Row>
            )}
            {b.restart_at && <Row label="Restarts at">{time(b.restart_at)}</Row>}
          </dl>
        </Card>
        <Card title="Commands">
          <CommandForm account={account} level={levelOf(b)} spots={spots.data} />
          {spots.error && <Failed error={spots.error} />}
        </Card>
        <PvPCard bot={b} />
      </div>
      <InspectCard account={account} />
      <Card title="Log">
        {logs.error ? <Failed error={logs.error} retry={() => void logs.refetch()} /> : <LogView lines={logs.data ?? []} />}
      </Card>
    </div>
  );
}
