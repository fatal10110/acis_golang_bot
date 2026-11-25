import { useMemo } from "react";
import { Chart, type Series } from "../components/Chart";
import type { Confirm } from "../components/ConfirmDialog";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Card } from "../components/ui/Card";
import { Loading } from "../components/ui/States";
import { api } from "../lib/api";
import { time } from "../lib/format";
import type { Sample } from "../lib/history";
import type { Dashboard as DashboardData } from "../lib/types";
import { useRun } from "../lib/useRun";

function Stat({ label, value, hint }: { label: string; value: string | number; hint?: string }) {
  return (
    <Card>
      <div className="text-xs text-slate-500 uppercase">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums">{value}</div>
      {hint && <div className="mt-1 text-xs text-slate-500">{hint}</div>}
    </Card>
  );
}

export function Dashboard({ dash, samples, ask }: { dash: DashboardData | undefined; samples: Sample[]; ask: (c: Confirm) => void }) {
  const { run, busy } = useRun();
  const times = useMemo(() => samples.map((s) => s.t), [samples]);
  const online: Series[] = useMemo(() => [{ label: "running", color: "#10b981", values: samples.map((s) => s.running) }], [samples]);
  const traffic: Series[] = useMemo(
    () => [
      { label: "packets sent/s", color: "#0ea5e9", values: samples.map((s) => s.sent) },
      { label: "events/s", color: "#f59e0b", values: samples.map((s) => s.events) },
    ],
    [samples],
  );
  if (!dash) return <Loading what="the dashboard" />;
  const m = dash.metrics;
  const rules = Object.entries(m?.rules ?? {}).sort((a, b) => b[1] - a[1]);
  const plans = Object.entries(m?.nav_plans_total ?? {}).sort((a, b) => b[1] - a[1]);
  const planTime = m?.nav_plan_us;
  const planMean = planTime && planTime.count > 0 ? (planTime.sum / planTime.count / 1000).toFixed(1) : "—";
  const planMax = planTime && planTime.count > 0 ? (planTime.max / 1000).toFixed(1) : "—";
  const locates = Object.entries(m?.nav_locate_total ?? {}).sort((a, b) => b[1] - a[1]);
  const lruMB = ((m?.nav_lru_bytes ?? 0) / 1e6).toFixed(1);
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">Dashboard</h1>
      <Card title="Login gate">
        {dash.halted ? (
          <div role="alert" className="flex flex-wrap items-center gap-3">
            <Badge tone="red">logins halted</Badge>
            <span className="text-sm">{dash.halt_err}</span>
            <Button
              variant="danger"
              className="ml-auto"
              disabled={busy}
              onClick={() => {
                ask({
                  title: "Resume logins?",
                  body: "The credential breaker halted every login. Resume only once the cause is fixed: repeated failures risk an IP ban.",
                  action: "Resume logins",
                  danger: true,
                  run: () => void run("Resume logins", api.resumeLogins),
                });
              }}
            >
              Resume logins
            </Button>
          </div>
        ) : (
          <Badge tone="green">logins open</Badge>
        )}
      </Card>
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <Stat label="Online" value={`${dash.running} / ${dash.max_online}`} hint="running of the online cap" />
        <Stat label="Bots" value={dash.bots} hint="in the registry" />
        <Stat label="Reconnects" value={m?.reconnects ?? 0} />
        <Stat label="Server aborts" value={m?.aborts ?? 0} />
        <Stat label="Packets sent" value={m?.sent ?? 0} />
        <Stat label="Paced" value={m?.paced ?? 0} hint="sends held by a pacing gate" />
        <Stat label="Events" value={m?.events ?? 0} />
        <Stat label="Metrics at" value={(m && time(m.at)) || "—"} />
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <Chart title="Bots running" times={times} series={online} />
        </Card>
        <Card>
          <Chart title="Traffic" times={times} series={traffic} />
        </Card>
      </div>
      <Card title="Route plans">
        <p className="mb-2 text-sm text-slate-500">
          {planTime?.count ?? 0} plans, mean {planMean} ms, max {planMax} ms
        </p>
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-slate-500 uppercase">
              <th scope="col">Outcome</th>
              <th scope="col" className="text-right">
                Plans
              </th>
            </tr>
          </thead>
          <tbody>
            {plans.map(([k, v]) => (
              <tr key={k} className="border-t border-slate-100 dark:border-slate-800">
                <td className="py-1">{k}</td>
                <td className="py-1 text-right tabular-nums">{v}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
      <Card title="Nav cache">
        <p className="mb-2 text-sm text-slate-500">
          LRU {lruMB} MB, {m?.nav_places ?? 0} places kept
        </p>
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-slate-500 uppercase">
              <th scope="col">Answered</th>
              <th scope="col" className="text-right">
                Locates
              </th>
            </tr>
          </thead>
          <tbody>
            {locates.map(([k, v]) => (
              <tr key={k} className="border-t border-slate-100 dark:border-slate-800">
                <td className="py-1">{k}</td>
                <td className="py-1 text-right tabular-nums">{v}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
      <Card title="Rule hits">
        {rules.length === 0 ? (
          <p className="text-sm text-slate-500">No rule ran yet.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 uppercase">
                <th scope="col">Rule</th>
                <th scope="col" className="text-right">
                  Hits
                </th>
              </tr>
            </thead>
            <tbody>
              {rules.map(([k, v]) => (
                <tr key={k} className="border-t border-slate-100 dark:border-slate-800">
                  <td className="py-1">{k}</td>
                  <td className="py-1 text-right tabular-nums">{v}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  );
}
