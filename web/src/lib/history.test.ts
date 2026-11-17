import { describe, expect, it } from "vitest";
import { History, MAX_SAMPLES } from "./history";
import type { Metrics, Update } from "./types";

function update(sec: number, sent: number, events: number, running: number, metrics = true): Update {
  const m = { at: new Date(sec * 1000).toISOString(), sent, events } as Metrics;
  return { bots: [], dashboard: { halted: false, bots: 0, running, max_online: 5, metrics: metrics ? m : null } };
}

describe("History", () => {
  it("turns counters into per-second rates", () => {
    const h = new History();
    const zero = update(0, 0, 0, 0);
    if (zero.dashboard.metrics) zero.dashboard.metrics.at = "0001-01-01T00:00:00Z";
    expect(h.add(zero)).toEqual([]);
    expect(h.add(update(100, 0, 0, 1))).toEqual([]);
    expect(h.add(update(100, 50, 50, 1))).toEqual([]);
    expect(h.add(update(102, 20, 40, 2, false))).toEqual([]);
    expect(h.add(update(102, 20, 40, 2))).toEqual([{ t: 102, running: 2, sent: 10, events: 20 }]);
    // A counter that went back (a new registry) rates 0, never negative.
    expect(h.add(update(104, 0, 60, 3)).at(-1)).toEqual({ t: 104, running: 3, sent: 0, events: 10 });
  });

  it("keeps the latest samples only", () => {
    const h = new History();
    for (let i = 0; i <= MAX_SAMPLES + 10; i++) h.add(update(i * 2, i, i, 1));
    expect(h.samples).toHaveLength(MAX_SAMPLES);
    expect(h.samples.at(-1)?.t).toBe((MAX_SAMPLES + 10) * 2);
  });
});
