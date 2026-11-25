import type { Bot, ClassInfo, RunState } from "./types";

export type ClassNames = ReadonlyMap<number, string>;

export function classMap(classes: readonly ClassInfo[] | undefined): ClassNames {
  return new Map((classes ?? []).map((c) => [c.id, c.name]));
}

/** classOf is the bot's class id: the live one, else the registry's. */
export function classOf(b: Bot): number | undefined {
  return b.live && b.live.class_id > 0 ? b.live.class_id : b.class;
}

export function className(names: ClassNames, id: number | undefined): string {
  if (id === undefined) return "";
  return names.get(id) ?? `class ${id}`;
}

export function levelOf(b: Bot): number {
  return b.live?.level || b.level || 0;
}

export function characterOf(b: Bot): string {
  return b.name || b.live?.character || "";
}

/** Status is the badge a bot shows: its run state, or online/offline. */
export type Status = RunState | "online" | "offline";

export function statusOf(b: Bot): Status {
  if (b.run !== "none") return b.run;
  return b.online ? "online" : "offline";
}

export function whereOf(b: Bot): string {
  if (b.live?.town) return `near town (${b.live.town.x}, ${b.live.town.y})`;
  const p = b.live?.position ?? b.position;
  return p ? `${p.x}, ${p.y}, ${p.z}` : "";
}

/** partyOf is the bot's party slot, its pin and its party mode. */
export function partyOf(b: Bot): string {
  const parts = [b.live?.party ?? ""];
  if (b.party_group) parts.push(b.party_lead ? `leads pin ${b.party_group}` : `pin ${b.party_group}`);
  if (b.party_mode === "off") parts.push("party off");
  return parts.filter(Boolean).join(" / ");
}

/** activityOf is the rule the bot runs and the order it follows. */
export function activityOf(b: Bot): string {
  return [b.live?.rule, b.live?.order].filter(Boolean).join(" / ");
}

export function percent(v: number, max: number): number {
  if (max <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((v / max) * 100)));
}

export function time(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  // Go's zero time marks "never".
  return Number.isNaN(d.getTime()) || d.getTime() <= 0 ? "" : d.toLocaleTimeString();
}

export type SortKey = "account" | "character" | "class" | "level" | "preset" | "status" | "hp";

/** sortValue is what the bots table sorts a column by. */
export function sortValue(b: Bot, key: SortKey, names: ClassNames): string | number {
  switch (key) {
    case "account":
      return b.account;
    case "character":
      return characterOf(b);
    case "class":
      return className(names, classOf(b));
    case "level":
      return levelOf(b);
    case "preset":
      return b.preset;
    case "status":
      return statusOf(b);
    case "hp":
      return b.live ? percent(b.live.hp, b.live.max_hp) : -1;
  }
}

export function sortBots(bots: readonly Bot[], key: SortKey, desc: boolean, names: ClassNames): Bot[] {
  const dir = desc ? -1 : 1;
  return [...bots].sort((a, b) => {
    const x = sortValue(a, key, names);
    const y = sortValue(b, key, names);
    const c =
      typeof x === "number" && typeof y === "number"
        ? x - y
        : String(x).localeCompare(String(y), undefined, { numeric: true });
    return c !== 0 ? c * dir : a.account.localeCompare(b.account, undefined, { numeric: true });
  });
}

/** matches reports whether the bot matches a free-text filter. */
export function matches(b: Bot, filter: string, names: ClassNames): boolean {
  const f = filter.trim().toLowerCase();
  if (!f) return true;
  return [b.account, characterOf(b), b.preset, className(names, classOf(b)), statusOf(b), activityOf(b)].some((s) =>
    s.toLowerCase().includes(f),
  );
}
