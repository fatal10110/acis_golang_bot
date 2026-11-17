import * as T from "@radix-ui/react-toast";
import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import { cn } from "../lib/cn";

export type ToastTone = "ok" | "info" | "error";

export interface ToastMessage {
  title: string;
  description?: string;
  tone?: ToastTone;
}

interface Shown extends ToastMessage {
  id: number;
}

const Ctx = createContext<(m: ToastMessage) => void>(() => undefined);

/** useToast returns the function that shows a toast. */
export function useToast() {
  return useContext(Ctx);
}

const tones: Record<ToastTone, string> = {
  ok: "border-emerald-500",
  info: "border-sky-500",
  error: "border-rose-500",
};

let nextId = 1;

/** Toaster shows the answers of the panel's requests. */
export function Toaster({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Shown[]>([]);
  const show = useCallback((m: ToastMessage) => {
    setToasts((ts) => [...ts.slice(-4), { ...m, id: nextId++ }]);
  }, []);
  const drop = (id: number) => {
    setToasts((ts) => ts.filter((t) => t.id !== id));
  };
  return (
    <Ctx.Provider value={show}>
      <T.Provider duration={5000} swipeDirection="right">
        {children}
        {toasts.map((t) => (
          <T.Root
            key={t.id}
            onOpenChange={(open) => {
              if (!open) drop(t.id);
            }}
            type={t.tone === "error" ? "foreground" : "background"}
            className={cn(
              "rounded-md border-l-4 bg-white p-3 text-sm shadow-lg dark:bg-slate-800",
              tones[t.tone ?? "info"],
            )}
          >
            <div className="flex items-start gap-2">
              <div className="flex-1">
                <T.Title className="font-semibold">{t.title}</T.Title>
                {t.description && <T.Description className="mt-0.5 text-slate-600 dark:text-slate-300">{t.description}</T.Description>}
              </div>
              <T.Close aria-label="Dismiss" className="rounded px-1 text-slate-500 hover:text-slate-900 dark:hover:text-white">
                ×
              </T.Close>
            </div>
          </T.Root>
        ))}
        <T.Viewport className="fixed right-4 bottom-4 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2 outline-none" />
      </T.Provider>
    </Ctx.Provider>
  );
}
