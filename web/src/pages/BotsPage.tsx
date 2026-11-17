import { useState } from "react";
import { BotsTable } from "../components/BotsTable";
import type { Confirm } from "../components/ConfirmDialog";
import { CreateDialog } from "../components/CreateDialog";
import { ImportDialog } from "../components/ImportDialog";
import { useToast } from "../components/Toaster";
import { Button } from "../components/ui/Button";
import { Empty, Failed, Loading } from "../components/ui/States";
import { api } from "../lib/api";
import { useClassNames, usePresets } from "../lib/queries";
import type { Bot } from "../lib/types";

/**
 * runBulk sends one request per bot, in order: the supervisor queues each
 * spawn at the login gate, which paces the logins.
 */
async function runBulk(action: "spawn" | "despawn", accounts: string[]) {
  const failed: string[] = [];
  for (const a of accounts) {
    try {
      await api.act(a, action);
    } catch (e) {
      failed.push(`${a}: ${e instanceof Error ? e.message : String(e)}`);
    }
  }
  return failed;
}

export function BotsPage({ bots, ask }: { bots: Bot[] | undefined; ask: (c: Confirm) => void }) {
  const presets = usePresets();
  const classes = useClassNames();
  const toast = useToast();
  const [dialog, setDialog] = useState<"create" | "import" | null>(null);
  const close = () => {
    setDialog(null);
  };

  const onBulk = (action: "spawn" | "despawn", accounts: string[]) => {
    ask({
      title: `${action === "spawn" ? "Spawn" : "Despawn"} ${accounts.length} bots?`,
      body:
        action === "spawn"
          ? "Each bot logs in through the login gate, which paces logins; bots over the online cap are refused."
          : "Each bot logs out and stays out until spawned again.",
      action: action === "spawn" ? "Spawn" : "Despawn",
      danger: action === "despawn",
      run: () => {
        void runBulk(action, accounts).then((failed) => {
          toast(
            failed.length === 0
              ? { title: `${action}: ${accounts.length} bots`, description: "done", tone: "ok" }
              : { title: `${action}: ${failed.length} of ${accounts.length} refused`, description: failed.slice(0, 5).join("\n"), tone: "error" },
          );
        });
      },
    });
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="mr-auto text-xl font-semibold">Bots</h1>
        <Button variant="primary" disabled={!presets.data || !classes.data} onClick={() => { setDialog("create"); }}>
          Create bot
        </Button>
        <Button onClick={() => { setDialog("import"); }}>Import</Button>
      </div>
      {presets.error && <Failed error={presets.error} retry={() => void presets.refetch()} />}
      {bots === undefined ? (
        <Loading what="bots" />
      ) : bots.length === 0 ? (
        <Empty>No bots yet. Create one, or import accounts made by `bots provision`.</Empty>
      ) : (
        <BotsTable bots={bots} names={classes.names} onBulk={onBulk} />
      )}
      {dialog === "create" && presets.data && classes.data && (
        <CreateDialog open onClose={close} presets={presets.data.presets} classes={classes.data} />
      )}
      {dialog === "import" && <ImportDialog open onClose={close} />}
    </div>
  );
}
