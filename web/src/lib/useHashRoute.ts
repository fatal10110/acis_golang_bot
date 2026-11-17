import { useSyncExternalStore } from "react";

/** Route is a page of the panel, from the location hash. */
export type Route =
  | { page: "bots" }
  | { page: "bot"; account: string }
  | { page: "dashboard" }
  | { page: "logs" };

export function parseRoute(hash: string): Route {
  const h = hash.replace(/^#\/?/, "");
  if (h.startsWith("bot/")) {
    const account = decodeURIComponent(h.slice(4));
    if (account) return { page: "bot", account };
  }
  if (h === "dashboard") return { page: "dashboard" };
  if (h === "logs") return { page: "logs" };
  return { page: "bots" };
}

export function hrefOf(r: Route): string {
  return r.page === "bot" ? `#/bot/${encodeURIComponent(r.account)}` : `#/${r.page}`;
}

function subscribe(cb: () => void) {
  window.addEventListener("hashchange", cb);
  return () => {
    window.removeEventListener("hashchange", cb);
  };
}

/** useHashRoute is the page the location hash names. */
export function useHashRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash);
  return parseRoute(hash);
}
