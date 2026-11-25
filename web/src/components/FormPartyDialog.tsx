import { useState, type SyntheticEvent } from "react";
import { api } from "../lib/api";
import { className, type ClassNames } from "../lib/format";
import type { PartyBot } from "../lib/types";
import { useRun } from "../lib/useRun";
import { Button } from "./ui/Button";
import { Dialog } from "./ui/Dialog";
import { Field, Select } from "./ui/Field";

/** maxMembers is the most bots that join a leader (botparty.MaxSize - 1). */
export const maxMembers = 3;

/** botLabel names a party bot: character, class, level and preset. */
export function botLabel(b: PartyBot, names: ClassNames): string {
  const who = b.character ?? b.account;
  const cls = className(names, b.class_id);
  return `${who} (${[cls, b.level ? `lv ${b.level}` : "", b.role ?? b.preset].filter(Boolean).join(", ")})`;
}

/**
 * FormPartyDialog forms a party by hand: a leader, one to three members and
 * whether the party is a pin, from the free pool and the bots of other
 * parties (each leaves its party). A pin holds whatever
 * level band, composition or break say until dissolved; an auto party needs
 * a farmer leader and follows every rule of the organizer.
 */
export function FormPartyDialog({
  open,
  onClose,
  bots,
  names,
  auto,
}: {
  open: boolean;
  onClose: (formed: boolean) => void;
  bots: readonly PartyBot[];
  names: ClassNames;
  auto: boolean;
}) {
  const [leader, setLeader] = useState("");
  const [members, setMembers] = useState<string[]>([]);
  const [pin, setPin] = useState(!auto);
  const { run, busy } = useRun();
  const close = (formed: boolean) => {
    setLeader("");
    setMembers([]);
    setPin(!auto);
    onClose(formed);
  };
  const toggle = (account: string) => {
    setMembers((m) => (m.includes(account) ? m.filter((a) => a !== account) : [...m, account]));
  };
  const lead = bots.find((b) => b.account === leader);
  const autoLeader = !lead || lead.preset === "farmer";
  const ready = !!lead && members.length >= 1 && members.length <= maxMembers && (pin || (auto && autoLeader));
  const submit = async (e: SyntheticEvent) => {
    e.preventDefault();
    const ok = await run("Form party", () => api.formParty(leader, members, pin));
    if (ok) close(true);
  };
  return (
    <Dialog
      open={open}
      onClose={() => {
        close(false);
      }}
      title="Form a party"
    >
      <form className="space-y-4" onSubmit={(e) => void submit(e)}>
        <Field label="Leader" hint="An auto party's leader is a farmer (Damage).">
          <Select
            value={leader}
            required
            onChange={(e) => {
              const v = e.target.value;
              setLeader(v);
              setMembers((m) => m.filter((a) => a !== v));
            }}
          >
            <option value="">choose a bot</option>
            {bots.map((b) => (
              <option key={b.account} value={b.account}>
                {botLabel(b, names)}
              </option>
            ))}
          </Select>
        </Field>
        <fieldset className="space-y-1">
          <legend className="text-sm font-medium">Members (1 to {maxMembers})</legend>
          {bots
            .filter((b) => b.account !== leader)
            .map((b) => (
              <label key={b.account} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={members.includes(b.account)}
                  disabled={!members.includes(b.account) && members.length >= maxMembers}
                  onChange={() => {
                    toggle(b.account);
                  }}
                />
                {botLabel(b, names)}
              </label>
            ))}
        </fieldset>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={pin}
            onChange={(e) => {
              setPin(e.target.checked);
            }}
          />
          Pin: keep it whatever level band, composition or breaks say, over restarts, until dissolved
        </label>
        {!pin && !auto && <p className="text-sm text-amber-700 dark:text-amber-400">Auto parties are off: only a pin forms.</p>}
        {!pin && auto && !autoLeader && (
          <p className="text-sm text-amber-700 dark:text-amber-400">An auto party needs a farmer leader.</p>
        )}
        <div className="flex justify-end gap-2">
          <Button
            onClick={() => {
              close(false);
            }}
          >
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !ready}>
            Form
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
