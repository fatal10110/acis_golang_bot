import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Bot } from "../lib/types";
import { bot, live } from "../test/fixtures";
import { PvPCard } from "./PvPCard";
import { Toaster } from "./Toaster";

const pvp = {
  fight_back: true,
  duels: true,
  hunt: false,
  arenas: true,
  pk: false,
  pvp_kills: 7,
  pk_kills: 2,
  duels_won: 3,
  duels_lost: 1,
  duels_tied: 0,
  last_duel: "won",
};

function answers() {
  const calls: { url: string; method?: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method, body: init?.body ? (JSON.parse(init.body as string) as unknown) : undefined });
      return Promise.resolve(new Response(JSON.stringify({ answer: "done" }), { status: 200 }));
    }),
  );
  return calls;
}

function show(b: Bot) {
  render(
    <Toaster>
      <PvPCard bot={b} />
    </Toaster>,
  );
}

describe("PvPCard", () => {
  it("shows the modes, kill counts and duels of the live snapshot", () => {
    show(bot("hz1", { online: true, run: "running", live: live({ pvp }) }));
    expect(screen.getByRole("checkbox", { name: "Duels" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Hunt" })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Arenas" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "PK" })).not.toBeChecked();
    expect(screen.getByText("preset defaults")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("3 won, 1 lost, 0 tied")).toBeInTheDocument();
    expect(screen.getByText("won")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Defaults" })).toBeDisabled();
  });

  it("flips one switch and posts all four", async () => {
    const calls = answers();
    const user = userEvent.setup();
    show(bot("hz1", { online: true, run: "running", live: live({ pvp }) }));
    await user.click(screen.getByRole("checkbox", { name: "Hunt" }));
    expect(calls).toEqual([
      { url: "/api/bots/hz1/pvp", method: "POST", body: { modes: { duels: true, hunt: true, arenas: true, pk: false } } },
    ]);
  });

  it("prefers the panel's switches and resets them to the defaults", async () => {
    const calls = answers();
    const user = userEvent.setup();
    show(bot("hz1", { pvp: { duels: false, hunt: false, arenas: false, pk: true } }));
    expect(screen.getByRole("checkbox", { name: "PK" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Duels" })).not.toBeChecked();
    expect(screen.getByText("set in the panel")).toBeInTheDocument();
    expect(screen.getByText("shown after the bot's first session")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Defaults" }));
    expect(calls).toEqual([{ url: "/api/bots/hz1/pvp", method: "POST", body: { modes: null } }]);
  });

  it("disables the switches until the bot's first session reports its defaults", () => {
    show(bot("hz1"));
    for (const name of ["Duels", "Hunt", "Arenas", "PK"]) {
      expect(screen.getByRole("checkbox", { name })).toBeDisabled();
    }
    expect(screen.getByText("preset defaults, shown after the bot's first session")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Defaults" })).toBeDisabled();
  });
});
