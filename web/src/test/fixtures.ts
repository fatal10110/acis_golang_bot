import type { Bot, Live } from "../lib/types";

export function live(over: Partial<Live> = {}): Live {
  return {
    at: "2026-11-17T10:00:00Z",
    state: "online",
    class_id: 0,
    level: 1,
    hp: 50,
    max_hp: 100,
    mp: 20,
    max_mp: 40,
    cp: 10,
    max_cp: 10,
    position: { x: 1, y: 2, z: 3 },
    reconnects: 0,
    ...over,
  };
}

export function bot(account: string, over: Partial<Bot> = {}): Bot {
  return { account, prefix: "hz", preset: "support", online: false, run: "none", ...over };
}
