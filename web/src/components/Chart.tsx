import { useEffect, useRef } from "react";
import uPlot from "uplot";

export interface Series {
  label: string;
  color: string;
  values: number[];
}

/** Chart draws series over time (seconds since the epoch) with uPlot. */
export function Chart({ title, times, series }: { title: string; times: number[]; series: Series[] }) {
  const box = useRef<HTMLDivElement>(null);
  const plot = useRef<uPlot | null>(null);
  const labels = series.map((s) => `${s.label}:${s.color}`).join(",");

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const p = new uPlot(
      {
        width: el.clientWidth || 400,
        height: 180,
        legend: { show: true },
        cursor: { drag: { x: false, y: false } },
        series: [{}, ...series.map((s) => ({ label: s.label, stroke: s.color, width: 2 }))],
        axes: [{ stroke: "#94a3b8" }, { stroke: "#94a3b8" }],
      },
      [[], ...series.map(() => [])],
      el,
    );
    plot.current = p;
    const ro = new ResizeObserver(() => {
      p.setSize({ width: el.clientWidth, height: 180 });
    });
    ro.observe(el);
    return () => {
      ro.disconnect();
      p.destroy();
      plot.current = null;
    };
    // The plot is rebuilt only when its series change, not on new data.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [labels]);

  useEffect(() => {
    plot.current?.setData([times, ...series.map((s) => s.values)]);
  }, [times, series]);

  return (
    <figure>
      <figcaption className="mb-1 text-sm font-semibold text-slate-600 dark:text-slate-400">{title}</figcaption>
      <div ref={box} role="img" aria-label={title} />
    </figure>
  );
}
