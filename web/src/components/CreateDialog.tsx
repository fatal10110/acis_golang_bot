import { useState, type SyntheticEvent } from "react";
import { api } from "../lib/api";
import type { ClassInfo, Preset } from "../lib/types";
import { useRun } from "../lib/useRun";
import { Button } from "./ui/Button";
import { Dialog } from "./ui/Dialog";
import { Field, Select } from "./ui/Field";

/** PresetOptions lists the presets; a disabled one shows why. */
export function PresetOptions({ presets }: { presets: readonly Preset[] }) {
  return presets.map((p) => (
    <option key={p.name} value={p.name} disabled={!!p.disabled} title={p.disabled ?? p.help}>
      {p.disabled ? `${p.name} (${p.disabled})` : p.name}
    </option>
  ));
}

export function firstOffered(presets: readonly Preset[]): string {
  return presets.find((p) => !p.disabled)?.name ?? "";
}

export function CreateDialog({
  open,
  onClose,
  presets,
  classes,
}: {
  open: boolean;
  onClose: () => void;
  presets: readonly Preset[];
  classes: readonly ClassInfo[];
}) {
  const [preset, setPreset] = useState(() => firstOffered(presets));
  const [cls, setCls] = useState("");
  const { run, busy } = useRun();
  const chosen = presets.find((p) => p.name === preset);
  const submit = async (e: SyntheticEvent) => {
    e.preventDefault();
    const ok = await run("Create bot", () => api.create(preset, cls === "" ? null : Number(cls)));
    if (ok) onClose();
  };
  return (
    <Dialog open={open} onClose={onClose} title="Create a bot">
      <form className="space-y-4" onSubmit={(e) => void submit(e)}>
        <Field label="Preset" hint={chosen?.help}>
          <Select
            value={preset}
            required
            onChange={(e) => {
              setPreset(e.target.value);
            }}
          >
            <PresetOptions presets={presets} />
          </Select>
        </Field>
        <Field label="Starting class" hint="A new account and character are created; the bot logs in at the gate's pace.">
          <Select
            value={cls}
            onChange={(e) => {
              setCls(e.target.value);
            }}
          >
            <option value="">random</option>
            {classes
              .filter((c) => c.starting)
              .map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
          </Select>
        </Field>
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !preset}>
            Create
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
