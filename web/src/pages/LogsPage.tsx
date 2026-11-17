import { useMemo, useState } from "react";
import { LogView } from "../components/LogView";
import { Button } from "../components/ui/Button";
import { Card } from "../components/ui/Card";
import { Input, Select } from "../components/ui/Field";
import { Failed, Loading } from "../components/ui/States";
import { useLogs } from "../lib/queries";
import type { Bot } from "../lib/types";

const levels = ["", "DEBUG", "INFO", "WARN", "ERROR"] as const;
const rank: Record<string, number> = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 };

export function LogsPage({ bots }: { bots: Bot[] | undefined }) {
  const [account, setAccount] = useState("");
  const [level, setLevel] = useState<(typeof levels)[number]>("");
  const [search, setSearch] = useState("");
  const [paused, setPaused] = useState(false);
  const logs = useLogs(account, 1000, paused);
  const lines = useMemo(() => {
    const q = search.trim().toLowerCase();
    const min = level ? (rank[level] ?? 0) : -1;
    return (logs.data ?? []).filter((l) => (rank[l.level] ?? 1) >= min && (!q || l.text.toLowerCase().includes(q)));
  }, [logs.data, level, search]);
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">Logs</h1>
      <Card>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Select
            aria-label="Bot"
            value={account}
            onChange={(e) => {
              setAccount(e.target.value);
            }}
            className="w-auto"
          >
            <option value="">all bots</option>
            {(bots ?? []).map((b) => (
              <option key={b.account} value={b.account}>
                {b.account}
              </option>
            ))}
          </Select>
          <Select
            aria-label="Lowest level"
            value={level}
            onChange={(e) => {
              setLevel(e.target.value as (typeof levels)[number]);
            }}
            className="w-auto"
          >
            {levels.map((l) => (
              <option key={l} value={l}>
                {l ? `${l} and above` : "any level"}
              </option>
            ))}
          </Select>
          <Input
            type="search"
            aria-label="Search the log"
            placeholder="Search"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
            }}
            className="max-w-xs"
          />
          <Button
            aria-pressed={paused}
            onClick={() => {
              setPaused((p) => !p);
            }}
          >
            {paused ? "Resume" : "Pause"}
          </Button>
          <span className="text-sm text-slate-500">{lines.length} lines</span>
        </div>
        {logs.error ? (
          <Failed error={logs.error} retry={() => void logs.refetch()} />
        ) : logs.isPending ? (
          <Loading what="log lines" />
        ) : (
          <LogView lines={lines} follow={!paused} className="h-[60vh]" />
        )}
      </Card>
    </div>
  );
}
