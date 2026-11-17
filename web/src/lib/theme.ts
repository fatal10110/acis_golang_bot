import { useCallback, useState } from "react";

export type Theme = "light" | "dark";

const KEY = "bots-ui-theme";

function initial(): Theme {
  const saved = localStorage.getItem(KEY);
  if (saved === "light" || saved === "dark") return saved;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function applyTheme(t: Theme) {
  document.documentElement.classList.toggle("dark", t === "dark");
}

/** useTheme is the page's theme, saved in the browser. */
export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(() => {
    const t = initial();
    applyTheme(t);
    return t;
  });
  const toggle = useCallback(() => {
    setTheme((t) => {
      const next = t === "dark" ? "light" : "dark";
      localStorage.setItem(KEY, next);
      applyTheme(next);
      return next;
    });
  }, []);
  return [theme, toggle];
}
