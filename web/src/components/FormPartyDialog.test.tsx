import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { PartyBot } from "../lib/types";
import { FormPartyDialog } from "./FormPartyDialog";
import { Toaster } from "./Toaster";

const names = new Map([
  [0, "Human Fighter"],
  [10, "Human Mystic"],
]);

const bot = (account: string, character: string, preset: string, cls: number): PartyBot => ({
  account,
  character,
  class_id: cls,
  level: 20,
  preset,
  online: true,
});

const bots = [
  bot("hz1", "Farm", "farmer", 0),
  bot("hz2", "Heal", "support", 10),
  bot("hz3", "Sup", "support", 10),
  bot("hz4", "Buff", "support", 10),
  bot("hz5", "Extra", "support", 10),
];

function answers(...res: [number, unknown][]) {
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      bodies.push({ url, method: init?.method, body: JSON.parse(init?.body as string) as unknown });
      const [status, body] = res.shift() ?? [500, { error: "no answer" }];
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
    }),
  );
  return bodies;
}

function show(auto = true) {
  const onClose = vi.fn();
  render(
    <Toaster>
      <FormPartyDialog open bots={bots} names={names} auto={auto} onClose={onClose} />
    </Toaster>,
  );
  return onClose;
}

describe("FormPartyDialog", () => {
  it("forms an auto party: leader, members, no pin", async () => {
    const user = userEvent.setup();
    const bodies = answers([201, { id: 4 }]);
    const onClose = show();
    expect(screen.getByRole("button", { name: "Form" })).toBeDisabled();
    await user.selectOptions(screen.getByRole("combobox", { name: /^Leader/ }), "hz1");
    expect(screen.queryByRole("checkbox", { name: /^Farm/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: "Heal (Human Mystic, lv 20, support)" }));
    await user.click(screen.getByRole("checkbox", { name: /^Sup/ }));
    expect(screen.getByRole("checkbox", { name: /^Pin/ })).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "Form" }));
    expect(await screen.findByText("done")).toBeInTheDocument();
    expect(bodies).toEqual([{ url: "/api/parties", method: "POST", body: { leader: "hz1", members: ["hz2", "hz3"], pin: false } }]);
    expect(onClose).toHaveBeenCalledWith(true);
  });

  it("takes at most three members", async () => {
    const user = userEvent.setup();
    show();
    await user.selectOptions(screen.getByRole("combobox", { name: /^Leader/ }), "hz1");
    for (const n of [/^Heal/, /^Sup/, /^Buff/]) await user.click(screen.getByRole("checkbox", { name: n }));
    expect(screen.getByRole("checkbox", { name: /^Extra/ })).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: /^Buff/ }));
    expect(screen.getByRole("checkbox", { name: /^Extra/ })).toBeEnabled();
  });

  it("needs a pin for a support leader and toasts a refusal", async () => {
    const user = userEvent.setup();
    const bodies = answers([409, { error: "party refused: hz3 is offline" }]);
    const onClose = show();
    await user.selectOptions(screen.getByRole("combobox", { name: /^Leader/ }), "hz2");
    await user.click(screen.getByRole("checkbox", { name: /^Sup/ }));
    expect(screen.getByText("An auto party needs a farmer leader.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Form" })).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: /^Pin/ }));
    await user.click(screen.getByRole("button", { name: "Form" }));
    expect(await screen.findByText("party refused: hz3 is offline")).toBeInTheDocument();
    expect(bodies).toEqual([{ url: "/api/parties", method: "POST", body: { leader: "hz2", members: ["hz3"], pin: true } }]);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("offers only a pin while auto parties are off", () => {
    show(false);
    expect(screen.getByRole("checkbox", { name: /^Pin/ })).toBeChecked();
  });

  it("reads the auto switch afresh on each opening", () => {
    const dialog = (open: boolean, auto: boolean) => (
      <Toaster>
        <FormPartyDialog open={open} bots={bots} names={names} auto={auto} onClose={vi.fn()} />
      </Toaster>
    );
    const { rerender } = render(dialog(false, true));
    rerender(dialog(false, false));
    rerender(dialog(true, false));
    expect(screen.getByRole("checkbox", { name: /^Pin/ })).toBeChecked();
  });
});
