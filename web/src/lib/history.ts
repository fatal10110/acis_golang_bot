import type { Update } from "./types";

/** Sample is one point of the dashboard charts. */
export interface Sample {
  /** Seconds since the epoch. */
  t: number;
  running: number;
  /** Packets sent and events handled per second since the last sample. */
  sent: number;
  events: number;
}

/** MAX_SAMPLES keeps ten minutes of 2 s updates. */
export const MAX_SAMPLES = 300;

interface Last {
  t: number;
  sent: number;
  events: number;
}

/**
 * History folds updates into chart samples: rates come from the metrics'
 * counters between two collections, and an update whose metrics were not
 * collected again adds no sample.
 */
export class History {
  private last: Last | null = null;
  samples: Sample[] = [];

  add(u: Update): Sample[] {
    const m = u.dashboard.metrics;
    if (!m) return this.samples;
    const t = new Date(m.at).getTime() / 1000;
    // The zero time: no collection yet.
    if (Number.isNaN(t) || t <= 0 || (this.last && t <= this.last.t)) return this.samples;
    const prev = this.last;
    this.last = { t, sent: m.sent, events: m.events };
    if (!prev) return this.samples;
    const dt = t - prev.t;
    const rate = (now: number, then: number) => (now >= then ? (now - then) / dt : 0);
    const next = [...this.samples, { t, running: u.dashboard.running, sent: rate(m.sent, prev.sent), events: rate(m.events, prev.events) }];
    this.samples = next.length > MAX_SAMPLES ? next.slice(next.length - MAX_SAMPLES) : next;
    return this.samples;
  }
}
