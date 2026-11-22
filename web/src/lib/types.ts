// The panel's JSON API (internal/botpanel/api.go). No field carries a
// secret: passwords derive from the secret file, which the panel never sees.

export type RunState = "none" | "running" | "stopping" | "halted" | "cooling down";

export interface Point {
  x: number;
  y: number;
  z: number;
}

/** A bot's live snapshot (botclient.Status). */
export interface Live {
  at: string;
  state: string;
  character?: string;
  class_id: number;
  level: number;
  hp: number;
  max_hp: number;
  mp: number;
  max_mp: number;
  cp: number;
  max_cp: number;
  position: Point;
  town?: Point;
  preset?: string;
  rule?: string;
  leader?: string;
  members?: string[];
  order?: string;
  reconnects: number;
  err?: string;
  crafter?: Crafter;
}

/** A crafter's status (botclient.CrafterStatus); times are RFC 3339. */
export interface Crafter {
  create_item: number;
  recipes: number;
  book_known: boolean;
  orders: number;
  claimed: number;
  phase: string;
  chains: number;
  chains_done: number;
  next_check?: string;
  last_cycle?: string;
  last_cycle_at?: string;
  breaker_until?: string;
}

/** A bot of the registry, its run and its live snapshot. */
export interface Bot {
  account: string;
  prefix: string;
  name?: string;
  class?: number;
  never_create?: boolean;
  preset: string;
  online: boolean;
  level?: number;
  position?: Point;
  stopped_at?: string;
  stop_reason?: string;
  run: RunState;
  restart_at?: string;
  live?: Live;
}

export interface Histogram {
  count: number;
  sum: number;
  max: number;
  /** Only the non-empty buckets, each with the largest value it holds (le) and its count (n). */
  buckets: { le: number; n: number }[];
}

export interface Metrics {
  /** The zero time ("0001-01-01T00:00:00Z") until the first collection. */
  at: string;
  bots: number;
  events: number;
  sent: number;
  paced: number;
  reconnects: number;
  aborts: number;
  breaker_open: boolean;
  rules: Record<string, number> | null;
  queue_depth: Histogram;
  wake_lag_us: Histogram;
}

export interface Dashboard {
  halted: boolean;
  halt_err?: string;
  bots: number;
  running: number;
  max_online: number;
  metrics: Metrics | null;
}

/** What /api/events sends as "update" every few seconds. */
export interface Update {
  bots: Bot[];
  dashboard: Dashboard;
}

export interface Preset {
  name: string;
  help: string;
  /** Why the panel does not offer it; absent when it does. */
  disabled?: string;
}

export interface Presets {
  presets: Preset[];
  classes: number[];
}

export interface ClassInfo {
  id: number;
  name: string;
  starting: boolean;
}

export interface Spot {
  name: string;
  min_level: number;
  max_level: number;
  aggro_level: number;
  cap: number;
  held: number;
}

export interface LogLine {
  at: string;
  level: string;
  account?: string;
  text: string;
}

export interface Answer {
  answer: string;
}

/** asUpdate checks an event stream message has an update's shape. */
export function asUpdate(raw: unknown): Update | null {
  if (typeof raw !== "object" || raw === null) return null;
  const u = raw as Partial<Record<keyof Update, unknown>>;
  if (!Array.isArray(u.bots) || typeof u.dashboard !== "object" || u.dashboard === null) return null;
  return raw as Update;
}
