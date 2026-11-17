import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Spot } from "../lib/types";
import { CommandForm } from "./CommandForm";
import { Toaster } from "./Toaster";

const spots: Spot[] = [
  { name: "Orcs", min_level: 20, max_level: 25, aggro_level: 0, cap: 2, held: 0 },
  { name: "Wolves", min_level: 3, max_level: 5, aggro_level: 4, cap: 3, held: 1 },
  { name: "Full", min_level: 3, max_level: 4, aggro_level: 0, cap: 1, held: 1 },
];

/** answers stubs fetch with one response per call, and records the bodies. */
function answers(...res: [number, unknown][]) {
  const bodies: unknown[] = [];
  const fetch = vi.fn((url: string, init?: RequestInit) => {
    bodies.push({ url, method: init?.method, body: JSON.parse(init?.body as string) as unknown });
    const [status, body] = res.shift() ?? [500, { error: "no answer" }];
    return Promise.resolve(new Response(JSON.stringify(body), { status }));
  });
  vi.stubGlobal("fetch", fetch);
  return bodies;
}

function show(s: readonly Spot[] | undefined = spots) {
  render(
    <Toaster>
      <CommandForm account="hz1" level={4} spots={s} />
    </Toaster>,
  );
}

describe("CommandForm", () => {
  it("sends a plain command and toasts the answer", async () => {
    const user = userEvent.setup();
    const bodies = answers([200, { answer: "done" }], [202, { answer: "queued" }]);
    show();
    await user.click(screen.getByRole("button", { name: "stay" }));
    expect(await screen.findByText("done")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "logout" }));
    expect(await screen.findByText("queued")).toBeInTheDocument();
    expect(bodies).toEqual([
      { url: "/api/bots/hz1/command", method: "POST", body: { command: "stay" } },
      { url: "/api/bots/hz1/command", method: "POST", body: { command: "logout" } },
    ]);
  });

  it("picks a spot, fitting ones first, full ones disabled, and toasts a refusal", async () => {
    const user = userEvent.setup();
    const bodies = answers([422, { error: "that spot is full" }]);
    show();
    const picker = screen.getByRole("combobox", { name: "Spot" });
    const options = Array.from(picker.querySelectorAll("option")).map((o) => o.value);
    expect(options).toEqual(["", "Full", "Wolves", "Orcs"]);
    expect(screen.getByRole("option", { name: /^Full/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: "go" })).toBeDisabled();
    await user.type(screen.getByRole("searchbox", { name: "Search spots" }), "wol");
    await user.selectOptions(picker, "Wolves");
    await user.click(screen.getByRole("button", { name: "go" }));
    expect(await screen.findByText("that spot is full")).toBeInTheDocument();
    expect(bodies).toEqual([{ url: "/api/bots/hz1/command", method: "POST", body: { command: "go Wolves" } }]);
  });

  it("follows and assists a named player", async () => {
    const user = userEvent.setup();
    const bodies = answers([200, { answer: "done" }], [200, { answer: "done" }]);
    show();
    await user.type(screen.getByRole("textbox", { name: "Player name" }), " Gm Name ");
    await user.click(screen.getByRole("button", { name: "follow" }));
    await user.click(screen.getByRole("button", { name: "assist" }));
    await screen.findAllByText("done");
    expect(bodies.map((b) => (b as { body: unknown }).body)).toEqual([{ command: "follow Gm Name" }, { command: "assist Gm Name" }]);
  });

  it("says when there are no spots", () => {
    answers();
    show([]);
    expect(screen.getByText(/start the panel with -datapack/)).toBeInTheDocument();
  });
});
