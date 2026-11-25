import type { Answer, Bot, ClassInfo, Inspection, LogLine, Parties, Presets, Spot } from "./types";

/** ApiError is a refused request: its HTTP status and the panel's reason. */
export class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null;
}

/** request sends a JSON request and returns the answer, or throws ApiError. */
export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: { Accept: "application/json" } };
  if (body !== undefined) {
    init.headers = { Accept: "application/json", "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  const data: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const reason = isRecord(data) && typeof data.error === "string" ? data.error : res.statusText;
    throw new ApiError(res.status, reason || `HTTP ${res.status}`);
  }
  return data as T;
}

const bot = (account: string) => `/api/bots/${encodeURIComponent(account)}`;

export type BotAction = "spawn" | "despawn" | "respawn";

export const api = {
  presets: () => request<Presets>("GET", "/api/presets"),
  classes: () => request<ClassInfo[]>("GET", "/api/classes"),
  spots: () => request<Spot[]>("GET", "/api/spots"),
  logs: (account: string, n: number) =>
    request<LogLine[]>("GET", `/api/logs?n=${n}&account=${encodeURIComponent(account)}`),
  create: (preset: string, cls: number | null) =>
    request<Bot>("POST", "/api/bots", cls === null ? { preset } : { preset, class: cls }),
  importBots: (prefix: string, count: number, marker: string, markerAt: string) =>
    request<{ added: number }>("POST", "/api/import", { prefix, count, marker, marker_at: markerAt }),
  act: (account: string, action: BotAction) => request<Answer>("POST", `${bot(account)}/${action}`),
  remove: (account: string) => request<Answer>("DELETE", bot(account)),
  setPreset: (account: string, preset: string) => request<Answer>("POST", `${bot(account)}/preset`, { preset }),
  command: (account: string, command: string) => request<Answer>("POST", `${bot(account)}/command`, { command }),
  inspect: (account: string) => request<Inspection>("GET", `${bot(account)}/inspect`),
  resumeLogins: () => request<Answer>("POST", "/api/resume-logins"),
  parties: () => request<Parties>("GET", "/api/parties"),
  formParty: (leader: string, members: string[], pin: boolean) =>
    request<{ id: number }>("POST", "/api/parties", { leader, members, pin }),
  dissolveParty: (id: number) => request<Answer>("DELETE", `/api/parties/${id}`),
  setPartyAuto: (on: boolean) => request<Answer>("POST", "/api/parties/auto", { on }),
  setPartyMode: (account: string, mode: "auto" | "off") => request<Answer>("POST", `${bot(account)}/party`, { mode }),
};
