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
  /** The party slot the organizer gave: "lead A B", "join L A"; absent for none. */
  party?: string;
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
  /** "off" keeps the bot out of bot parties; absent takes part. */
  party_mode?: string;
  /** The pinned party the bot belongs to, and whether it leads it. */
  party_group?: string;
  party_lead?: boolean;
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
  /** The walker's route plans by outcome (ok, budget, door, blocked, no_route, isolated, no_nav). */
  nav_plans_total: Record<string, number> | null;
  /** How long each route plan took, in microseconds. */
  nav_plan_us: Histogram;
}

export interface Dashboard {
  halted: boolean;
  halt_err?: string;
  bots: number;
  running: number;
  max_online: number;
  metrics: Metrics | null;
}

/** A bot of a party or of the free pool (botregistry.PartyBot). */
export interface PartyBot {
  account: string;
  character?: string;
  class_id?: number;
  level: number;
  preset: string;
  role?: string;
  online: boolean;
  slot?: string;
  town?: Point;
}

/** A bot party of the organizer (botregistry.PartyInfo); times are RFC 3339. */
export interface Party {
  id: number;
  leader: PartyBot;
  members: PartyBot[];
  /** The leader's party members that are players: names only. */
  players: string[];
  spot?: string;
  state: "forming" | "active" | "broken";
  why?: string;
  since: string;
  formed_at: string;
  pinned: boolean;
  group?: string;
}

/** GET /api/parties: the auto switch, every party and the bots free to group. */
export interface Parties {
  auto: boolean;
  parties: Party[];
  free: PartyBot[];
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

/** A carried item of an inspect; weight is the stack's, absent when unknown. */
export interface InspectItem {
  id: number;
  name: string;
  count: number;
  enchant?: number;
  weight?: number;
}

/** A worn item of an inspect; grade is "" when unknown. */
export interface WornItem {
  slot: string;
  id: number;
  name: string;
  enchant?: number;
  grade: string;
}

export interface Penalties {
  weight_level: number;
  grade: boolean;
  grade_weapon: boolean;
  grade_armor: number;
  expertise: number;
  death: number;
  run_speed: number;
  base_run_speed?: number;
}

export interface InspectEffect {
  skill_id: number;
  name: string;
  level: number;
  left_s: number;
  no_end?: boolean;
}

/** A merchant's held valuable: where its units are, its unsold rounds and the ask of one unit. */
export interface HeldValuable {
  id: number;
  name: string;
  enchant?: number;
  place: "carried" | "stored" | "carried, stored";
  carried: number;
  stored: number;
  rounds: number;
  ask: number;
  released?: boolean;
}

export type InspectState = "fresh" | "stale" | "offline" | "unresponsive";

/** An inspect of a bot (GET /api/bots/{account}/inspect): the state alone for an offline or unresponsive bot. */
export interface Inspection {
  state: InspectState;
  at?: string;
  inventory?: { items: InspectItem[]; load: number; max_load: number; percent: number };
  equipment?: WornItem[];
  penalties?: Penalties;
  effects?: InspectEffect[];
  valuables?: HeldValuable[];
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
