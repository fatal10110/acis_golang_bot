import { useQuery } from "@tanstack/react-query";
import { api } from "./api";
import { classMap } from "./format";

export function usePresets() {
  return useQuery({ queryKey: ["presets"], queryFn: api.presets, staleTime: Infinity });
}

/** useClassNames is the class names by id; empty until they load. */
export function useClassNames() {
  const q = useQuery({ queryKey: ["classes"], queryFn: api.classes, staleTime: Infinity });
  return { ...q, names: classMap(q.data) };
}

export function useSpots(enabled = true) {
  return useQuery({ queryKey: ["spots"], queryFn: api.spots, enabled, refetchInterval: 10_000 });
}

/** useLogs tails the log ring: one bot's lines, or all for "". */
export function useLogs(account: string, n: number, paused = false) {
  return useQuery({
    queryKey: ["logs", account, n],
    queryFn: () => api.logs(account, n),
    refetchInterval: paused ? false : 3_000,
  });
}
