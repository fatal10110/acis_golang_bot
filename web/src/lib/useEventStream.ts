import { useEffect, useRef, useState } from "react";

/** StreamStatus is the state of a server-sent event stream. */
export type StreamStatus = "connecting" | "live" | "reconnecting";

/** EventSourceLike is what the hook needs of an EventSource (a test seam). */
export interface EventSourceLike {
  readonly readyState: number;
  onopen: ((ev: Event) => void) | null;
  onerror: ((ev: Event) => void) | null;
  addEventListener(type: string, listener: (ev: MessageEvent<string>) => void): void;
  close(): void;
}

export interface StreamOptions {
  /** Opens a source; the browser's EventSource by default. */
  open?: (url: string) => EventSourceLike;
  /** A stream silent this long is reopened (the panel sends every 2 s). */
  staleMs?: number;
  /** First and longest wait before reopening a closed stream. */
  minBackoffMs?: number;
  maxBackoffMs?: number;
}

const CLOSED = 2;

/**
 * useEventStream follows a server-sent event stream and hands every `event`
 * message, parsed from JSON and checked by decode, to onMessage; a message
 * decode refuses (null) is dropped. A stream the browser gave up on,
 * or one silent for staleMs, is reopened after a backoff that doubles up to
 * maxBackoffMs and resets once a message arrives.
 */
export function useEventStream<T>(
  url: string,
  event: string,
  decode: (raw: unknown) => T | null,
  onMessage: (data: T) => void,
  opts: StreamOptions = {},
): StreamStatus {
  const [status, setStatus] = useState<StreamStatus>("connecting");
  const handler = useRef({ decode, onMessage });
  useEffect(() => {
    handler.current = { decode, onMessage };
  }, [decode, onMessage]);
  const { open = (u: string) => new EventSource(u), staleMs = 10_000, minBackoffMs = 1_000, maxBackoffMs = 30_000 } = opts;
  const openRef = useRef(open);

  useEffect(() => {
    let src: EventSourceLike | null = null;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let stale: ReturnType<typeof setTimeout> | undefined;
    let backoff = minBackoffMs;
    let done = false;

    const reopen = () => {
      src?.close();
      src = null;
      clearTimeout(stale);
      if (done) return;
      setStatus("reconnecting");
      clearTimeout(retry);
      retry = setTimeout(connect, backoff);
      backoff = Math.min(backoff * 2, maxBackoffMs);
    };
    const watch = () => {
      clearTimeout(stale);
      stale = setTimeout(reopen, staleMs);
    };
    function connect() {
      const s = openRef.current(url);
      src = s;
      watch();
      s.onopen = () => {
        setStatus("live");
        watch();
      };
      s.onerror = () => {
        if (s.readyState === CLOSED) {
          reopen();
          return;
        }
        // The browser retries on its own; show it.
        setStatus("reconnecting");
      };
      s.addEventListener(event, (ev) => {
        let data: T | null;
        try {
          data = handler.current.decode(JSON.parse(String(ev.data)));
        } catch {
          return;
        }
        if (data === null) return;
        backoff = minBackoffMs;
        setStatus("live");
        watch();
        handler.current.onMessage(data);
      });
    }
    connect();
    return () => {
      done = true;
      clearTimeout(retry);
      clearTimeout(stale);
      src?.close();
    };
  }, [url, event, staleMs, minBackoffMs, maxBackoffMs]);

  return status;
}
