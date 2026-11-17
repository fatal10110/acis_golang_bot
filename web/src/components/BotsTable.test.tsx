import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { bot, live } from "../test/fixtures";
import { BotsTable } from "./BotsTable";

const names = new Map([
  [0, "Human Fighter"],
  [18, "Elven Fighter"],
  [88, "Duelist"],
]);

const bots = [
  bot("hz1", { name: "Alden", online: true, run: "running", live: live({ class_id: 88, level: 78, rule: "farm", order: "go Wolves" }) }),
  bot("hz2", { class: 18, level: 12, stop_reason: "despawned" }),
  bot("hz10", { name: "Brin", online: true, run: "cooling down", class: 0, level: 40 }),
];

function accounts() {
  return screen
    .getAllByRole("row")
    .slice(1)
    .map((r) => within(r).getAllByRole("link")[0]?.textContent);
}

describe("BotsTable", () => {
  it("shows class names, badges and gauges", () => {
    render(<BotsTable bots={bots} names={names} onBulk={vi.fn()} />);
    const table = within(screen.getByRole("table"));
    expect(table.getByText("Duelist")).toBeInTheDocument();
    expect(table.getByText("Elven Fighter")).toBeInTheDocument();
    expect(table.getByText("running")).toBeInTheDocument();
    expect(table.getByText("cooling down")).toBeInTheDocument();
    expect(table.getByText("offline")).toBeInTheDocument();
    expect(table.getByText("despawned")).toBeInTheDocument();
    expect(screen.getByRole("meter", { name: "HP" })).toHaveAttribute("aria-valuetext", "50 / 100");
    expect(screen.getByText("farm / go Wolves")).toBeInTheDocument();
  });

  it("sorts by a column, both ways, accounts in natural order", async () => {
    const user = userEvent.setup();
    render(<BotsTable bots={bots} names={names} onBulk={vi.fn()} />);
    expect(accounts()).toEqual(["hz1", "hz2", "hz10"]);
    await user.click(screen.getByRole("button", { name: /Lv/ }));
    expect(accounts()).toEqual(["hz2", "hz10", "hz1"]);
    await user.click(screen.getByRole("button", { name: /Lv/ }));
    expect(accounts()).toEqual(["hz1", "hz10", "hz2"]);
    expect(screen.getByRole("columnheader", { name: /Lv/ })).toHaveAttribute("aria-sort", "descending");
  });

  it("filters by text and status", async () => {
    const user = userEvent.setup();
    render(<BotsTable bots={bots} names={names} onBulk={vi.fn()} />);
    await user.type(screen.getByRole("searchbox", { name: "Filter bots" }), "elven");
    expect(accounts()).toEqual(["hz2"]);
    await user.clear(screen.getByRole("searchbox", { name: "Filter bots" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Status" }), "cooling down");
    expect(accounts()).toEqual(["hz10"]);
  });

  it("runs a bulk action on the selected shown bots", async () => {
    const user = userEvent.setup();
    // The confirmation runs the action at once.
    const onBulk = vi.fn((_action: string, _accounts: string[], done: () => void) => {
      done();
    });
    render(<BotsTable bots={bots} names={names} onBulk={onBulk} />);
    const spawn = screen.getByRole("button", { name: "Spawn selected" });
    expect(spawn).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: "Select hz2" }));
    await user.click(screen.getByRole("checkbox", { name: "Select hz10" }));
    await user.click(spawn);
    expect(onBulk).toHaveBeenCalledWith("spawn", ["hz2", "hz10"], expect.any(Function));
    expect(spawn).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: "Select all shown bots" }));
    await user.click(screen.getByRole("button", { name: "Despawn selected" }));
    expect(onBulk).toHaveBeenLastCalledWith("despawn", ["hz1", "hz2", "hz10"], expect.any(Function));
  });

  it("keeps the selection when the bulk confirmation is cancelled", async () => {
    const user = userEvent.setup();
    // The confirmation is cancelled: done is never called.
    const onBulk = vi.fn();
    render(<BotsTable bots={bots} names={names} onBulk={onBulk} />);
    await user.click(screen.getByRole("checkbox", { name: "Select hz2" }));
    const spawn = screen.getByRole("button", { name: "Spawn selected" });
    await user.click(spawn);
    expect(onBulk).toHaveBeenCalledTimes(1);
    expect(spawn).toBeEnabled();
    expect(screen.getByRole("checkbox", { name: "Select hz2" })).toBeChecked();
  });
});
