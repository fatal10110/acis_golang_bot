import { useMemo, useState } from "react";
import type { Spot } from "../lib/types";
import { Input, Select } from "./ui/Field";

/** spotLabel names a spot with its levels and how full it is. */
export function spotLabel(s: Spot): string {
  const aggro = s.aggro_level > 0 ? `, aggro ≤${s.aggro_level}` : "";
  return `${s.name} (lv ${s.min_level}-${s.max_level}${aggro}, ${s.held}/${s.cap} bots)`;
}

/**
 * SpotPicker chooses a farming spot: a search over names, the spots that
 * fit level first (by distance of their levels from it).
 */
export function SpotPicker({
  spots,
  level,
  value,
  onChange,
}: {
  spots: readonly Spot[];
  level: number;
  value: string;
  onChange: (name: string) => void;
}) {
  const [q, setQ] = useState("");
  const shown = useMemo(() => {
    const f = q.trim().toLowerCase();
    const dist = (s: Spot) => (level < s.min_level ? s.min_level - level : level > s.max_level ? level - s.max_level : 0);
    return spots
      .filter((s) => !f || s.name.toLowerCase().includes(f))
      .sort((a, b) => dist(a) - dist(b) || a.min_level - b.min_level || a.name.localeCompare(b.name))
      .slice(0, 200);
  }, [spots, q, level]);
  return (
    <div className="flex flex-1 flex-col gap-1 sm:flex-row">
      <Input
        type="search"
        aria-label="Search spots"
        placeholder="Search spots"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
        }}
        className="sm:max-w-40"
      />
      <Select
        aria-label="Spot"
        value={value}
        required
        onChange={(e) => {
          onChange(e.target.value);
        }}
      >
        <option value="">choose a spot ({shown.length})</option>
        {shown.map((s) => (
          <option key={s.name} value={s.name} disabled={s.held >= s.cap}>
            {spotLabel(s)}
          </option>
        ))}
      </Select>
    </div>
  );
}
