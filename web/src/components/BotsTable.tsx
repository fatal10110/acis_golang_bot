import { useMemo, useState } from "react";
import { cn } from "../lib/cn";
import {
  activityOf,
  characterOf,
  className,
  classOf,
  levelOf,
  matches,
  sortBots,
  statusOf,
  whereOf,
  type ClassNames,
  type SortKey,
  type Status,
} from "../lib/format";
import { hrefOf } from "../lib/useHashRoute";
import type { Bot } from "../lib/types";
import { StatusBadge } from "./ui/Badge";
import { Bar } from "./ui/Bar";
import { Button } from "./ui/Button";
import { Input, Select } from "./ui/Field";

const statuses: Status[] = ["running", "online", "cooling down", "stopping", "halted", "offline"];

const columns: { key: SortKey; label: string; className?: string }[] = [
  { key: "account", label: "Account" },
  { key: "character", label: "Character" },
  { key: "class", label: "Class", className: "hidden md:table-cell" },
  { key: "level", label: "Lv" },
  { key: "hp", label: "HP / MP / CP" },
  { key: "preset", label: "Preset", className: "hidden lg:table-cell" },
  { key: "status", label: "Status" },
];

export interface BotsTableProps {
  bots: readonly Bot[];
  names: ClassNames;
  /**
   * Asks for a bulk action on the selected accounts. The caller calls done once the action
   * runs (after its confirmation), so a cancelled confirmation keeps the selection.
   */
  onBulk: (action: "spawn" | "despawn", accounts: string[], done: () => void) => void;
}

/** BotsTable lists the bots: sortable, filtered, selectable for bulk spawn and despawn. */
export function BotsTable({ bots, names, onBulk }: BotsTableProps) {
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "account", desc: false });
  const [filter, setFilter] = useState("");
  const [status, setStatus] = useState<Status | "">("");
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());

  const shown = useMemo(
    () =>
      sortBots(
        bots.filter((b) => matches(b, filter, names) && (status === "" || statusOf(b) === status)),
        sort.key,
        sort.desc,
        names,
      ),
    [bots, filter, status, sort, names],
  );
  // Selection only counts bots still shown.
  const chosen = shown.filter((b) => selected.has(b.account)).map((b) => b.account);
  const allChosen = shown.length > 0 && chosen.length === shown.length;

  const toggle = (account: string) => {
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(account)) n.delete(account);
      else n.add(account);
      return n;
    });
  };
  const bulk = (action: "spawn" | "despawn") => {
    onBulk(action, chosen, () => {
      setSelected(new Set());
    });
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-2">
        <Input
          type="search"
          aria-label="Filter bots"
          placeholder="Filter by account, name, class, preset…"
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
          }}
          className="max-w-xs"
        />
        <Select
          aria-label="Status"
          value={status}
          onChange={(e) => {
            setStatus(e.target.value as Status | "");
          }}
          className="w-auto"
        >
          <option value="">any status</option>
          {statuses.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </Select>
        <span className="text-sm text-slate-500" aria-live="polite">
          {shown.length} of {bots.length} bots{chosen.length > 0 && `, ${chosen.length} selected`}
        </span>
        <div className="ml-auto flex gap-2">
          <Button size="sm" disabled={chosen.length === 0} onClick={() => { bulk("spawn"); }}>
            Spawn selected
          </Button>
          <Button size="sm" disabled={chosen.length === 0} onClick={() => { bulk("despawn"); }}>
            Despawn selected
          </Button>
        </div>
      </div>
      <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-800">
        <table className="w-full text-left text-sm">
          <thead className="bg-slate-100 text-xs text-slate-600 uppercase dark:bg-slate-900 dark:text-slate-400">
            <tr>
              <th scope="col" className="w-8 px-3 py-2">
                <input
                  type="checkbox"
                  aria-label="Select all shown bots"
                  checked={allChosen}
                  onChange={() => {
                    setSelected(allChosen ? new Set() : new Set(shown.map((b) => b.account)));
                  }}
                />
              </th>
              {columns.map((c) => {
                const active = sort.key === c.key;
                return (
                  <th
                    key={c.key}
                    scope="col"
                    className={cn("px-3 py-2", c.className)}
                    aria-sort={active ? (sort.desc ? "descending" : "ascending") : "none"}
                  >
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 uppercase hover:text-slate-900 dark:hover:text-white"
                      onClick={() => {
                        setSort({ key: c.key, desc: active ? !sort.desc : false });
                      }}
                    >
                      {c.label}
                      <span aria-hidden="true">{active ? (sort.desc ? "▼" : "▲") : ""}</span>
                    </button>
                  </th>
                );
              })}
              <th scope="col" className="hidden px-3 py-2 xl:table-cell">
                Where
              </th>
              <th scope="col" className="hidden px-3 py-2 lg:table-cell">
                Rule / order
              </th>
            </tr>
          </thead>
          <tbody>
            {shown.map((b) => (
              <tr key={b.account} className="border-t border-slate-200 hover:bg-slate-50 dark:border-slate-800 dark:hover:bg-slate-900/60">
                <td className="px-3 py-2">
                  <input
                    type="checkbox"
                    aria-label={`Select ${b.account}`}
                    checked={selected.has(b.account)}
                    onChange={() => {
                      toggle(b.account);
                    }}
                  />
                </td>
                <td className="px-3 py-2 font-medium">
                  <a className="text-sky-700 hover:underline dark:text-sky-400" href={hrefOf({ page: "bot", account: b.account })}>
                    {b.account}
                  </a>
                </td>
                <td className="px-3 py-2">{characterOf(b)}</td>
                <td className="hidden px-3 py-2 md:table-cell">{className(names, classOf(b))}</td>
                <td className="px-3 py-2 tabular-nums">{levelOf(b) || ""}</td>
                <td className="px-3 py-2">
                  {b.live && b.live.max_hp > 0 ? (
                    <div className="space-y-0.5">
                      <Bar kind="hp" value={b.live.hp} max={b.live.max_hp} />
                      <Bar kind="mp" value={b.live.mp} max={b.live.max_mp} />
                      <Bar kind="cp" value={b.live.cp} max={b.live.max_cp} />
                    </div>
                  ) : (
                    <span className="text-slate-400">—</span>
                  )}
                </td>
                <td className="hidden px-3 py-2 lg:table-cell">{b.preset}</td>
                <td className="px-3 py-2">
                  <div className="flex flex-col items-start gap-0.5">
                    <StatusBadge status={statusOf(b)} />
                    {b.live && b.live.state !== "online" && <span className="text-xs text-slate-500">{b.live.state}</span>}
                    {b.stop_reason && !b.live && <span className="text-xs text-slate-500">{b.stop_reason}</span>}
                  </div>
                </td>
                <td className="hidden px-3 py-2 text-xs xl:table-cell">{whereOf(b)}</td>
                <td className="hidden px-3 py-2 text-xs lg:table-cell">{activityOf(b)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
