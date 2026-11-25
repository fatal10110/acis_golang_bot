import { useCallback, useRef, useState } from "react";
import { ConfirmDialog, type Confirm } from "./components/ConfirmDialog";
import { Badge } from "./components/ui/Badge";
import { Button } from "./components/ui/Button";
import { cn } from "./lib/cn";
import { History, type Sample } from "./lib/history";
import { useTheme } from "./lib/theme";
import { asUpdate, type Update } from "./lib/types";
import { useEventStream, type StreamStatus } from "./lib/useEventStream";
import { hrefOf, useHashRoute, type Route } from "./lib/useHashRoute";
import { BotDetail } from "./pages/BotDetail";
import { BotsPage } from "./pages/BotsPage";
import { Dashboard } from "./pages/Dashboard";
import { PartiesPage } from "./pages/PartiesPage";
import { LogsPage } from "./pages/LogsPage";

const nav: { route: Route; label: string }[] = [
  { route: { page: "bots" }, label: "Bots" },
  { route: { page: "parties" }, label: "Parties" },
  { route: { page: "dashboard" }, label: "Dashboard" },
  { route: { page: "logs" }, label: "Logs" },
];

function StreamBadge({ status }: { status: StreamStatus }) {
  const tone = status === "live" ? "green" : status === "reconnecting" ? "amber" : "slate";
  return (
    <span role="status" aria-live="polite">
      <Badge tone={tone}>{status === "reconnecting" ? "reconnecting…" : status}</Badge>
    </span>
  );
}

export function App() {
  const route = useHashRoute();
  const [theme, toggleTheme] = useTheme();
  const [update, setUpdate] = useState<Update>();
  const [samples, setSamples] = useState<Sample[]>([]);
  const history = useRef(new History());
  const [confirm, setConfirm] = useState<Confirm | null>(null);

  const onUpdate = useCallback((u: Update) => {
    setUpdate(u);
    setSamples(history.current.add(u));
  }, []);
  const status = useEventStream("/api/events", "update", asUpdate, onUpdate);
  const bots = update?.bots;
  const active = route.page === "bot" ? "bots" : route.page;

  return (
    <div className="min-h-screen">
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:p-2">
        Skip to content
      </a>
      <header className="sticky top-0 z-10 border-b border-slate-200 bg-white/90 backdrop-blur dark:border-slate-800 dark:bg-slate-950/90">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2">
          <strong className="text-lg">bots ui</strong>
          <nav aria-label="Pages" className="flex gap-1">
            {nav.map((n) => (
              <a
                key={n.label}
                href={hrefOf(n.route)}
                aria-current={active === n.route.page ? "page" : undefined}
                className={cn(
                  "rounded-md px-3 py-1.5 text-sm font-medium",
                  active === n.route.page ? "bg-slate-200 dark:bg-slate-800" : "hover:bg-slate-100 dark:hover:bg-slate-900",
                )}
              >
                {n.label}
              </a>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            {update?.dashboard.halted && <Badge tone="red">logins halted</Badge>}
            <StreamBadge status={status} />
            <Button variant="ghost" size="sm" onClick={toggleTheme} aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}>
              {theme === "dark" ? "☀" : "☾"}
            </Button>
          </div>
        </div>
      </header>
      <main id="main" className="mx-auto max-w-7xl px-4 py-6">
        {route.page === "bots" && <BotsPage bots={bots} ask={setConfirm} />}
        {route.page === "bot" && <BotDetail key={route.account} account={route.account} bots={bots} ask={setConfirm} />}
        {route.page === "parties" && <PartiesPage ask={setConfirm} />}
        {route.page === "dashboard" && <Dashboard dash={update?.dashboard} samples={samples} ask={setConfirm} />}
        {route.page === "logs" && <LogsPage bots={bots} />}
      </main>
      <ConfirmDialog
        confirm={confirm}
        onClose={() => {
          setConfirm(null);
        }}
      />
    </div>
  );
}
