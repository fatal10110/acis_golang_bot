import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { bot, live } from "../test/fixtures";
import { BotsTable } from "./BotsTable";

const names = new Map([[0, "Human Fighter"]]);

const bots = [
  bot("hz1", { online: true, run: "running", live: live({ rule: "farm", spot: "gludio_wolves" }) }),
  bot("hz2", { online: true, run: "running", live: live({ spot: "gludio_orcs", walk: "unreachable: no route" }) }),
  bot("hz3", { online: true, run: "running", live: live({ walk: "unreachable: blocked" }) }),
  bot("hz4", { online: true, run: "running", live: live() }),
];

function accounts() {
  return screen
    .getAllByRole("row")
    .slice(1)
    .map((r) => within(r).getAllByRole("link")[0]?.textContent);
}

describe("BotsTable spot and walk", () => {
  it("shows each online bot's spot and failed walk", () => {
    render(<BotsTable bots={bots} names={names} onBulk={vi.fn()} />);
    expect(screen.getByRole("columnheader", { name: "Spot / walk" })).toBeInTheDocument();
    expect(screen.getByText("gludio_wolves")).toBeInTheDocument();
    expect(screen.getByText("gludio_orcs / unreachable: no route")).toBeInTheDocument();
    expect(screen.getByText("unreachable: blocked")).toBeInTheDocument();
  });

  it("filters by spot and walk", async () => {
    const user = userEvent.setup();
    render(<BotsTable bots={bots} names={names} onBulk={vi.fn()} />);
    await user.type(screen.getByRole("searchbox", { name: "Filter bots" }), "unreachable");
    expect(accounts()).toEqual(["hz2", "hz3"]);
    await user.clear(screen.getByRole("searchbox", { name: "Filter bots" }));
    await user.type(screen.getByRole("searchbox", { name: "Filter bots" }), "wolves");
    expect(accounts()).toEqual(["hz1"]);
  });
});
