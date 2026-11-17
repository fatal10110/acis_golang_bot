import { useState, type SyntheticEvent } from "react";
import { api } from "../lib/api";
import { useRun } from "../lib/useRun";
import { Button } from "./ui/Button";
import { Dialog } from "./ui/Dialog";
import { Field, Input, Select } from "./ui/Field";

/** ImportDialog adds the accounts prefix1 to prefixN, made by `bots provision`. */
export function ImportDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [prefix, setPrefix] = useState("");
  const [count, setCount] = useState("1");
  const [marker, setMarker] = useState("");
  const [markerAt, setMarkerAt] = useState("suffix");
  const { run, busy } = useRun();
  const submit = async (e: SyntheticEvent) => {
    e.preventDefault();
    const ok = await run("Import bots", async () => {
      const r = await api.importBots(prefix.trim(), Number(count), marker.trim(), markerAt);
      return { answer: `${r.added} added` };
    });
    if (ok) onClose();
  };
  return (
    <Dialog open={open} onClose={onClose} title="Import bots">
      <form className="space-y-4" onSubmit={(e) => void submit(e)}>
        <Field label="Account prefix" hint="Imports accounts prefix1 to prefixN; their bots adopt existing characters and never create one.">
          <Input
            required
            value={prefix}
            autoComplete="off"
            onChange={(e) => {
              setPrefix(e.target.value);
            }}
          />
        </Field>
        <Field label="Accounts">
          <Input
            type="number"
            required
            min={1}
            max={1000}
            value={count}
            onChange={(e) => {
              setCount(e.target.value);
            }}
          />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name marker" hint="Empty for the default.">
            <Input
              value={marker}
              autoComplete="off"
              onChange={(e) => {
                setMarker(e.target.value);
              }}
            />
          </Field>
          <Field label="Marker placement">
            <Select
              value={markerAt}
              onChange={(e) => {
                setMarkerAt(e.target.value);
              }}
            >
              <option value="suffix">suffix</option>
              <option value="prefix">prefix</option>
            </Select>
          </Field>
        </div>
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !prefix.trim()}>
            Import
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
