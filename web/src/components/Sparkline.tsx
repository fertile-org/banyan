const BLOCKS = ["▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"];

interface SparklineProps {
  values: number[];
  max?: number;
}

export function Sparkline({ values, max = 100 }: SparklineProps) {
  if (values.length === 0) {
    return <span className="mono text-muted">-</span>;
  }

  const latest = values[values.length - 1] ?? 0;
  const colorClass =
    latest > 80 ? "sparkline-danger" : latest > 50 ? "sparkline-warning" : "";

  const chars = values.map((v) => {
    const idx = Math.min(Math.floor((v / max) * BLOCKS.length), BLOCKS.length - 1);
    return BLOCKS[Math.max(0, idx)] ?? BLOCKS[0];
  });

  return <span className={`sparkline ${colorClass}`}>{chars.join("")}</span>;
}

interface CPUCellProps {
  percent: number;
  history: number[];
}

/** Displays CPU as percentage number + sparkline history */
export function CPUCell({ percent, history }: CPUCellProps) {
  const color =
    percent > 80 ? "var(--red)" : percent > 50 ? "var(--yellow)" : "var(--text-secondary)";

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <span className="mono" style={{ color, fontSize: 12, minWidth: 38, textAlign: "right" }}>
        {percent.toFixed(1)}%
      </span>
      {history.length > 1 && <Sparkline values={history} />}
    </div>
  );
}
