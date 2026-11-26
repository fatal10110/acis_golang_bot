import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LoginSpacing } from "./LoginSpacing";
import { Toaster } from "./Toaster";

const dash = { login_spacing_ms: 1250, login_spacing_min_ms: 1250, login_spacing_max_ms: 60000 };

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

function show() {
  render(
    <Toaster>
      <LoginSpacing dash={dash} />
    </Toaster>,
  );
  return screen.getByRole("spinbutton", { name: /login spacing/i });
}

describe("LoginSpacing", () => {
  it("shows the spacing and its bounds, and bounds the field by them", () => {
    const field = show();
    expect(screen.getByText(/now 1250 ms/)).toBeInTheDocument();
    expect(screen.getByText(/from 1250 to 60000 ms/)).toBeInTheDocument();
    expect(field).toHaveAttribute("min", "1250");
    expect(field).toHaveAttribute("max", "60000");
    expect(screen.getByRole("button", { name: "Set" })).toBeDisabled();
  });

  it("sets the spacing and toasts the answer", async () => {
    const user = userEvent.setup();
    const calls = answers([200, { answer: "done" }]);
    const field = show();
    await user.type(field, "5000");
    await user.click(screen.getByRole("button", { name: "Set" }));
    expect(await screen.findByText("done")).toBeInTheDocument();
    expect(calls).toEqual([{ url: "/api/login-spacing", method: "POST", body: { ms: 5000 } }]);
    expect(field).toHaveValue(null);
  });

  it("toasts a refusal and keeps the value", async () => {
    const user = userEvent.setup();
    answers([400, { error: "login spacing out of bounds" }]);
    const field = show();
    // The browser's own min check is bypassed the way a stale page would.
    field.removeAttribute("min");
    await user.type(field, "900");
    await user.click(screen.getByRole("button", { name: "Set" }));
    expect(await screen.findByText(/out of bounds/)).toBeInTheDocument();
    expect(field).toHaveValue(900);
  });
});
