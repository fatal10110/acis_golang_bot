import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useEventStream, type EventSourceLike } from "./useEventStream";

class FakeSource implements EventSourceLike {
  readyState = 0;
  onopen: ((ev: Event) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  closed = false;
  private listeners = new Map<string, ((ev: MessageEvent<string>) => void)[]>();
  constructor(readonly url: string) {}
  addEventListener(type: string, l: (ev: MessageEvent<string>) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), l]);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  open() {
    this.readyState = 1;
    this.onopen?.(new Event("open"));
  }
  fail(readyState: number) {
    this.readyState = readyState;
    this.onerror?.(new Event("error"));
  }
  emit(type: string, data: string) {
    for (const l of this.listeners.get(type) ?? []) l(new MessageEvent(type, { data }));
  }
}

interface Msg {
  n: number;
}
const decode = (raw: unknown): Msg | null =>
  typeof raw === "object" && raw !== null && typeof (raw as Msg).n === "number" ? (raw as Msg) : null;

function setup() {
  const sources: FakeSource[] = [];
  const got: Msg[] = [];
  const open = (url: string) => {
    const s = new FakeSource(url);
    sources.push(s);
    return s;
  };
  const onMessage = (m: Msg) => {
    got.push(m);
  };
  const hook = renderHook(() =>
    useEventStream("/api/events", "update", decode, onMessage, { open, staleMs: 10_000, minBackoffMs: 1_000, maxBackoffMs: 4_000 }),
  );
  const last = () => {
    const s = sources.at(-1);
    if (!s) throw new Error("no source opened");
    return s;
  };
  return { hook, sources, got, last };
}

describe("useEventStream", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("goes live and hands over the decoded messages only", () => {
    const { hook, got, last } = setup();
    expect(hook.result.current).toBe("connecting");
    expect(last().url).toBe("/api/events");
    act(() => {
      last().open();
    });
    expect(hook.result.current).toBe("live");
    act(() => {
      last().emit("update", '{"n":1}');
      last().emit("update", "not json");
      last().emit("update", '{"other":2}');
      last().emit("ping", '{"n":3}');
      last().emit("update", '{"n":4}');
    });
    expect(got).toEqual([{ n: 1 }, { n: 4 }]);
  });

  it("shows the browser's own retry without opening another stream", () => {
    const { hook, sources, last } = setup();
    act(() => {
      last().open();
      last().fail(0);
    });
    expect(hook.result.current).toBe("reconnecting");
    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(sources).toHaveLength(1);
    act(() => {
      last().open();
    });
    expect(hook.result.current).toBe("live");
  });

  it("reopens a closed stream with a doubling backoff, reset by a message", () => {
    const { hook, sources, last } = setup();
    const steps = [1_000, 2_000, 4_000, 4_000];
    for (const [i, wait] of steps.entries()) {
      act(() => {
        last().fail(2);
      });
      expect(hook.result.current).toBe("reconnecting");
      act(() => {
        vi.advanceTimersByTime(wait - 1);
      });
      expect(sources, `step ${i}: reopened before ${wait} ms`).toHaveLength(i + 1);
      act(() => {
        vi.advanceTimersByTime(1);
      });
      expect(sources, `step ${i}: not reopened after ${wait} ms`).toHaveLength(i + 2);
      expect(sources[i]?.closed).toBe(true);
    }
    act(() => {
      last().open();
      last().emit("update", '{"n":1}');
      last().fail(2);
      vi.advanceTimersByTime(1_000);
    });
    expect(sources).toHaveLength(steps.length + 2);
  });

  it("reopens a stream that went silent", () => {
    const { hook, sources, last } = setup();
    act(() => {
      last().open();
      vi.advanceTimersByTime(9_000);
      last().emit("update", '{"n":1}');
      vi.advanceTimersByTime(9_000);
    });
    expect(sources).toHaveLength(1);
    act(() => {
      vi.advanceTimersByTime(1_000);
    });
    expect(hook.result.current).toBe("reconnecting");
    expect(sources[0]?.closed).toBe(true);
    act(() => {
      vi.advanceTimersByTime(1_000);
    });
    expect(sources).toHaveLength(2);
  });

  it("closes its stream on unmount and never reopens", () => {
    const { hook, sources, last } = setup();
    const s = last();
    act(() => {
      s.fail(2);
    });
    hook.unmount();
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(s.closed).toBe(true);
    expect(sources).toHaveLength(1);
  });
});
