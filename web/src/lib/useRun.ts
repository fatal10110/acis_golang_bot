import { useCallback, useState } from "react";
import { useToast } from "../components/Toaster";

/**
 * useRun runs a panel request and toasts its answer: "done", "queued" (the
 * bot answers on its next session) or the refusal. busy is set while one
 * runs.
 */
export function useRun() {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const run = useCallback(
    async (title: string, fn: () => Promise<unknown>): Promise<boolean> => {
      setBusy(true);
      try {
        const res = await fn();
        const answer = typeof res === "object" && res !== null && "answer" in res ? String(res.answer) : "done";
        toast({ title, description: answer, tone: answer === "queued" ? "info" : "ok" });
        return true;
      } catch (e) {
        toast({ title, description: e instanceof Error ? e.message : String(e), tone: "error" });
        return false;
      } finally {
        setBusy(false);
      }
    },
    [toast],
  );
  return { run, busy };
}
