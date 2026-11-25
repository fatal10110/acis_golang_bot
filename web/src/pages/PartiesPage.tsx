import { useState } from "react";
import type { Confirm } from "../components/ConfirmDialog";
import { botLabel, FormPartyDialog } from "../components/FormPartyDialog";
import { Badge, type Tone } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Card } from "../components/ui/Card";
import { Empty, Failed, Loading } from "../components/ui/States";
import { api } from "../lib/api";
import { time, type ClassNames } from "../lib/format";
import { useClassNames, useParties } from "../lib/queries";
import type { Parties, Party, PartyBot } from "../lib/types";
import { hrefOf } from "../lib/useHashRoute";
import { useRun } from "../lib/useRun";

const stateTones: Record<Party["state"], Tone> = { forming: "amber", active: "green", broken: "red" };

/** townOf names a free bot's town by its coordinates, "no town" for none. */
export function townOf(b: PartyBot): string {
  return b.town ? `town (${b.town.x}, ${b.town.y})` : "no town";
}

/** freeByTown groups the free pool by town, towns in name order. */
export function freeByTown(free: readonly PartyBot[]): [string, PartyBot[]][] {
  const m = new Map<string, PartyBot[]>();
  for (const b of free) {
    const t = townOf(b);
    m.set(t, [...(m.get(t) ?? []), b]);
  }
  return [...m.entries()].sort(([a], [b]) => a.localeCompare(b));
}

function BotLine({ b, names, lead }: { b: PartyBot; names: ClassNames; lead?: boolean }) {
  return (
    <li className="flex items-center gap-2 text-sm">
      <a className="text-sky-700 hover:underline dark:text-sky-400" href={hrefOf({ page: "bot", account: b.account })}>
        {botLabel(b, names)}
      </a>
      {lead && <Badge tone="sky">leader</Badge>}
      {!b.online && <Badge>offline</Badge>}
    </li>
  );
}

/** PartiesView shows the parties and the free pool and acts on them; changed refetches. */
export function PartiesView({
  data,
  names,
  ask,
  changed,
}: {
  data: Parties;
  names: ClassNames;
  ask: (c: Confirm) => void;
  changed: () => void;
}) {
  const { run, busy } = useRun();
  const [forming, setForming] = useState(false);
  const pickable = [
    ...data.free,
    ...data.parties.filter((p) => p.state !== "broken").flatMap((p) => [p.leader, ...p.members].filter((b) => b.online)),
  ];
  const act = (title: string, fn: () => Promise<unknown>) => {
    void run(title, fn).then((ok) => {
      if (ok) changed();
    });
  };
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="mr-auto text-xl font-semibold">Parties</h1>
        <span className="text-sm">Auto parties: {data.auto ? "on" : "off"}</span>
        <Button
          disabled={busy}
          onClick={() => {
            if (data.auto) {
              ask({
                title: "Switch auto parties off?",
                body: "Every auto party is dissolved on the next pass and none forms; pinned parties stay.",
                action: "Switch off",
                run: () => {
                  act("auto parties off", () => api.setPartyAuto(false));
                },
              });
            } else {
              act("auto parties on", () => api.setPartyAuto(true));
            }
          }}
        >
          {data.auto ? "Switch auto off" : "Switch auto on"}
        </Button>
        <Button
          variant="primary"
          disabled={pickable.length < 2}
          onClick={() => {
            setForming(true);
          }}
        >
          Form a party
        </Button>
      </div>
      {data.parties.length === 0 ? (
        <Empty>No bot party.</Empty>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {data.parties.map((p) => (
            <Card
              key={p.id}
              title={
                <span className="flex flex-wrap items-center gap-2">
                  Party {p.id}
                  <Badge tone={stateTones[p.state]}>{p.state}</Badge>
                  {p.pinned && <Badge tone="sky">pinned</Badge>}
                </span>
              }
            >
              <ul className="space-y-1" aria-label={`Party ${p.id} members`}>
                <BotLine b={p.leader} names={names} lead />
                {p.members.map((m) => (
                  <BotLine key={m.account} b={m} names={names} />
                ))}
                {p.players.map((n) => (
                  <li key={n} className="text-sm">
                    {n} <Badge>player</Badge>
                  </li>
                ))}
              </ul>
              <dl className="mt-2 grid grid-cols-3 gap-1 text-sm">
                <dt className="text-slate-500">Spot</dt>
                <dd className="col-span-2">{p.spot ?? "none"}</dd>
                <dt className="text-slate-500">Since</dt>
                <dd className="col-span-2">{time(p.since)}</dd>
                {p.why && (
                  <>
                    <dt className="text-slate-500">Why</dt>
                    <dd className="col-span-2">{p.why}</dd>
                  </>
                )}
              </dl>
              {p.state !== "broken" && (
                <div className="mt-3 flex justify-end">
                  <Button
                    variant="danger"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      ask({
                        title: `Dissolve party ${p.id}?`,
                        body: "Every member gets unparty on the next pass, the leader first; a pin's rows are cleared.",
                        action: "Dissolve",
                        danger: true,
                        run: () => {
                          act(`dissolve party ${p.id}`, () => api.dissolveParty(p.id));
                        },
                      });
                    }}
                  >
                    Dissolve
                  </Button>
                </div>
              )}
            </Card>
          ))}
        </div>
      )}
      <Card title="Free pool">
        {data.free.length === 0 ? (
          <Empty>No bot free to group.</Empty>
        ) : (
          freeByTown(data.free).map(([town, bots]) => (
            <section key={town} className="mb-2">
              <h3 className="text-sm font-medium">{town}</h3>
              <ul className="ml-2 space-y-1">
                {bots.map((b) => (
                  <BotLine key={b.account} b={b} names={names} />
                ))}
              </ul>
            </section>
          ))
        )}
      </Card>
      <FormPartyDialog
        open={forming}
        bots={pickable}
        names={names}
        auto={data.auto}
        onClose={(formed) => {
          setForming(false);
          if (formed) changed();
        }}
      />
    </div>
  );
}

export function PartiesPage({ ask }: { ask: (c: Confirm) => void }) {
  const q = useParties();
  const { names } = useClassNames();
  if (q.isPending) return <Loading what="parties" />;
  if (q.isError) return <Failed error={q.error} retry={() => void q.refetch()} />;
  return <PartiesView data={q.data} names={names} ask={ask} changed={() => void q.refetch()} />;
}
