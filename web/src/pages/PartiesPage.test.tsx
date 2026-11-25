import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Confirm } from "../components/ConfirmDialog";
import { Toaster } from "../components/Toaster";
import type { Parties } from "../lib/types";
import { freeByTown, PartiesView } from "./PartiesPage";

const names = new Map([
  [0, "Human Fighter"],
  [10, "Human Mystic"],
]);

const data: Parties = {
  auto: true,
  parties: [
    {
      id: 3,
      leader: { account: "hz1", character: "Farm", class_id: 0, level: 22, preset: "farmer", role: "damage", online: true, slot: "lead Heal" },
      members: [{ account: "hz2", character: "Heal", class_id: 10, level: 21, preset: "support", role: "support", online: true }],
      players: ["Pal"],
      spot: "Wolves",
      state: "active",
      since: "2026-11-26T10:00:00Z",
      formed_at: "2026-11-26T09:59:00Z",
      pinned: true,
      group: "Farm",
    },
    {
      id: 4,
      leader: { account: "hz5", character: "Gone", level: 0, preset: "farmer", online: false },
      members: [],
      players: [],
      state: "broken",
      why: "no healer",
      since: "2026-11-26T10:01:00Z",
      formed_at: "2026-11-26T09:00:00Z",
      pinned: false,
    },
  ],
  free: [
    { account: "hz3", character: "Sup", class_id: 10, level: 20, preset: "support", online: true, town: { x: 100, y: 200, z: 0 } },
    { account: "hz4", character: "Farm2", class_id: 0, level: 20, preset: "farmer", online: true },
  ],
};

function answers(...res: [number, unknown][]) {
  const calls: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method, body: init?.body ? (JSON.parse(init.body as string) as unknown) : undefined });
      const [status, body] = res.shift() ?? [500, { error: "no answer" }];
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
    }),
  );
  return calls;
}

function show(d: Parties = data) {
  const asked: Confirm[] = [];
  const changed = vi.fn();
  render(
    <Toaster>
      <PartiesView data={d} names={names} ask={(c) => asked.push(c)} changed={changed} />
    </Toaster>,
  );
  return { asked, changed };
}

describe("PartiesView", () => {
  it("shows each party's bots with class, level and role, its players, spot and state, and the free pool by town", () => {
    show();
    const party = screen.getByRole("list", { name: "Party 3 members" });
    expect(within(party).getByRole("link", { name: "Farm (Human Fighter, lv 22, damage)" })).toHaveAttribute("href", "#/bot/hz1");
    expect(within(party).getByRole("link", { name: "Heal (Human Mystic, lv 21, support)" })).toBeInTheDocument();
    expect(within(party).getByText("Pal")).toBeInTheDocument();
    expect(screen.getByText("Wolves")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
    expect(screen.getByText("pinned")).toBeInTheDocument();
    expect(screen.getByText("no healer")).toBeInTheDocument();
    // A broken party cannot be dissolved again.
    expect(screen.getAllByRole("button", { name: "Dissolve" })).toHaveLength(1);
    expect(screen.getByRole("heading", { name: "town (100, 200)" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "no town" })).toBeInTheDocument();
  });

  it("dissolves a party after a confirmation", async () => {
    const user = userEvent.setup();
    const calls = answers([200, { answer: "done" }]);
    const { asked, changed } = show();
    await user.click(screen.getByRole("button", { name: "Dissolve" }));
    expect(asked).toHaveLength(1);
    expect(asked[0]?.title).toBe("Dissolve party 3?");
    asked[0]?.run();
    expect(await screen.findByText("done")).toBeInTheDocument();
    expect(calls).toEqual([{ url: "/api/parties/3", method: "DELETE", body: undefined }]);
    expect(changed).toHaveBeenCalled();
  });

  it("switches auto parties off after a confirmation, on at once", async () => {
    const user = userEvent.setup();
    const calls = answers([200, { answer: "done" }], [200, { answer: "done" }]);
    const { asked } = show();
    await user.click(screen.getByRole("button", { name: "Switch auto off" }));
    asked[0]?.run();
    expect(await screen.findByText("done")).toBeInTheDocument();
    show({ ...data, auto: false });
    await user.click(screen.getByRole("button", { name: "Switch auto on" }));
    await vi.waitFor(() => {
      expect(calls).toEqual([
        { url: "/api/parties/auto", method: "POST", body: { on: false } },
        { url: "/api/parties/auto", method: "POST", body: { on: true } },
      ]);
    });
  });

  it("opens the form dialog with the free bots and the other parties' online bots", async () => {
    const user = userEvent.setup();
    show();
    await user.click(screen.getByRole("button", { name: "Form a party" }));
    const leader = screen.getByRole("combobox", { name: /^Leader/ });
    const options = Array.from(leader.querySelectorAll("option")).map((o) => o.value);
    expect(options).toEqual(["", "hz3", "hz4", "hz1", "hz2"]);
  });

  it("groups the free pool by town", () => {
    expect(freeByTown(data.free).map(([t, bs]) => [t, bs.map((b) => b.account)])).toEqual([
      ["no town", ["hz4"]],
      ["town (100, 200)", ["hz3"]],
    ]);
  });
});
