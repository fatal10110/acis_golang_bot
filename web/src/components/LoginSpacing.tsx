import { useState } from "react";
import { api } from "../lib/api";
import type { Dashboard } from "../lib/types";
import { useRun } from "../lib/useRun";
import { Button } from "./ui/Button";
import { Input } from "./ui/Field";

/**
 * LoginSpacing shows and sets the login gate's least time between two dial
 * starts. The panel refuses a value under the gate's floor (the server's
 * accept filter limit on a spaced gate), so the field only raises it.
 */
export function LoginSpacing({ dash }: { dash: Pick<Dashboard, "login_spacing_ms" | "login_spacing_min_ms" | "login_spacing_max_ms"> }) {
  const { run, busy } = useRun();
  const [ms, setMs] = useState("");
  const min = dash.login_spacing_min_ms;
  const max = dash.login_spacing_max_ms;
  return (
    <form
      className="flex flex-wrap items-end gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        const n = Number(ms);
        void run("Login spacing", () => api.setLoginSpacing(n)).then((ok) => {
          if (ok) setMs("");
        });
      }}
    >
      <label className="block space-y-1 text-sm">
        <span className="font-medium">Login spacing (ms)</span>
        <Input
          type="number"
          required
          min={min}
          max={max}
          step={1}
          placeholder={String(dash.login_spacing_ms)}
          value={ms}
          onChange={(e) => {
            setMs(e.target.value);
          }}
        />
        <span className="block text-xs text-slate-500">
          now {dash.login_spacing_ms} ms; every login and game dial starts at least this long after the previous one, from {min} to {max} ms
        </span>
      </label>
      <Button type="submit" disabled={busy || ms === ""}>
        Set
      </Button>
    </form>
  );
}
