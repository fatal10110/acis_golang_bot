import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { time } from "../lib/format";
import type { Crafter } from "../lib/types";
import { CrafterStatus } from "./CrafterStatus";

const at = "2026-11-22T10:00:00Z";
const hold = "2026-11-22T10:30:00Z";

function crafter(over: Partial<Crafter> = {}): Crafter {
  return { create_item: 1, recipes: 0, book_known: false, orders: 0, claimed: 0, phase: "idle", chains: 0, chains_done: 0, ...over };
}

function row(label: string): string | null | undefined {
  return screen.getByText(label).nextElementSibling?.textContent;
}

describe("CrafterStatus", () => {
  it("shows an idle crafter before any cycle", () => {
    render(
      <dl>
        <CrafterStatus crafter={crafter({ create_item: 0, recipes: 2, next_check: at })} />
      </dl>,
    );
    expect(row("Create Item")).toBe("not learned");
    expect(row("Recipes")).toBe("2 (book not read yet)");
    expect(row("Craft orders")).toBe("0 open, 0 claimed");
    expect(row("Craft cycle")).toBe(`idle, next check ${time(at)}`);
    expect(row("Last cycle")).toBe("none yet");
    expect(row("Breaker")).toBe("ok");
  });

  it("shows a crafter at work, its last cycle stopped by a breaker", () => {
    render(
      <dl>
        <CrafterStatus
          crafter={crafter({
            create_item: 3,
            recipes: 12,
            book_known: true,
            orders: 2,
            claimed: 1,
            phase: "make",
            chains: 3,
            chains_done: 1,
            last_cycle: "breaker tripped",
            last_cycle_at: at,
            breaker_until: hold,
          })}
        />
      </dl>,
    );
    expect(row("Create Item")).toBe("level 3");
    expect(row("Recipes")).toBe("12");
    expect(row("Craft orders")).toBe("2 open, 1 claimed");
    expect(row("Craft cycle")).toBe("make, 1 of 3 chains");
    expect(row("Last cycle")).toBe(`breaker tripped ${time(at)}`);
    expect(row("Breaker")).toBe(`tripped, holds until ${time(hold)}`);
  });
});
