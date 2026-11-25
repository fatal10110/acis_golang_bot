import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { time } from "../lib/format";
import type { Inspection } from "../lib/types";
import { InspectView } from "./InspectView";

const at = "2026-11-25T10:00:00Z";

function row(label: string): string | null | undefined {
  return screen.getByText(label).nextElementSibling?.textContent;
}

const fresh: Inspection = {
  state: "fresh",
  at,
  inventory: {
    items: [
      { id: 57, name: "item #57", count: 900 },
      { id: 1060, name: "Lesser Healing Potion", count: 14, weight: 70 },
    ],
    load: 13000,
    max_load: 20000,
    percent: 65,
  },
  equipment: [
    { slot: "weapon", id: 70, name: "Claymore", enchant: 3, grade: "C" },
    { slot: "head", id: 500, name: "item #500", grade: "" },
  ],
  penalties: {
    weight_level: 1,
    grade: true,
    grade_weapon: true,
    grade_armor: 3,
    expertise: 1,
    death: 2,
    run_speed: 97,
    base_run_speed: 115,
  },
  effects: [
    { skill_id: 1068, name: "Might", level: 1, left_s: 0, no_end: true },
    { skill_id: 1204, name: "Wind Walk", level: 2, left_s: 90 },
    { skill_id: 1040, name: "Shield", level: 3, left_s: 3720 },
  ],
};

describe("InspectView", () => {
  it("shows the inventory, gear, penalties and effects", () => {
    render(<InspectView inspection={fresh} />);
    expect(screen.getByText(`Taken ${time(at)}`)).toBeInTheDocument();
    expect(screen.getByText("Weight 13000 / 20000 (65%)")).toBeInTheDocument();
    const potion = screen.getByText("Lesser Healing Potion").closest("tr");
    expect(potion?.textContent).toBe("Lesser Healing Potion1470");
    expect(screen.getByText("item #57").closest("tr")?.textContent).toBe("item #57900?");
    expect(row("weapon")).toBe("+3 Claymore (C grade)");
    expect(row("head")).toBe("item #500");
    expect(row("Weight penalty")).toBe("level 1");
    expect(row("Grade penalty")).toBe("on: weapon, 3 armor pieces");
    expect(row("Death penalty")).toBe("level 2");
    expect(row("Run speed")).toBe("97 of base 115");
    expect(row("Might 1")).toBe("no end");
    expect(row("Wind Walk 2")).toBe("1m 30s");
    expect(row("Shield 3")).toBe("1h 2m");
  });

  it("marks a stale answer and shows no penalty as none", () => {
    render(
      <InspectView
        inspection={{
          ...fresh,
          state: "stale",
          equipment: [],
          effects: [],
          penalties: { weight_level: 0, grade: false, grade_weapon: false, grade_armor: 0, expertise: 0, death: 0, run_speed: 126 },
        }}
      />,
    );
    expect(screen.getByText(/The server sent no inventory in time/)).toBeInTheDocument();
    expect(screen.getByText("Nothing worn.")).toBeInTheDocument();
    expect(screen.getByText("No effects.")).toBeInTheDocument();
    expect(row("Weight penalty")).toBe("none");
    expect(row("Grade penalty")).toBe("none");
    expect(row("Death penalty")).toBe("none");
    expect(row("Run speed")).toBe("126");
  });

  it("shows a merchant's held valuables", () => {
    render(
      <InspectView
        inspection={{
          ...fresh,
          valuables: [
            { id: 6579, name: "Arcana Mace", enchant: 4, place: "carried", carried: 1, stored: 0, rounds: 1, ask: 900000 },
            { id: 1888, name: "Synthetic Cokes", place: "carried, stored", carried: 20, stored: 30, rounds: 3, ask: 4000, released: true },
            { id: 4039, name: "Mold Glue", place: "stored", carried: 0, stored: 12, rounds: 0, ask: 2500 },
          ],
        }}
      />,
    );
    expect(screen.getByText("Valuables")).toBeInTheDocument();
    expect(row("+4 Arcana Mace")).toBe("1 carried, 1 unsold round, ask 900000");
    expect(row("Synthetic Cokes")).toBe("20 carried, 30 stored, 3 unsold rounds, ask 4000, released to the store");
    expect(row("Mold Glue")).toBe("12 stored, 0 unsold rounds, ask 2500");
  });

  it("shows no valuables section for a bot holding none", () => {
    render(<InspectView inspection={fresh} />);
    expect(screen.queryByText("Valuables")).not.toBeInTheDocument();
  });

  it("shows an offline and an unresponsive bot", () => {
    const { rerender } = render(<InspectView inspection={{ state: "offline" }} />);
    expect(screen.getByText(/The bot is offline/)).toBeInTheDocument();
    rerender(<InspectView inspection={{ state: "unresponsive" }} />);
    expect(screen.getByText(/did not answer/)).toBeInTheDocument();
  });
});
