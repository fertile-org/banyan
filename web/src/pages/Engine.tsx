import { Cpu } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { formatBytes, formatRatio, timeAgo } from "@/lib/format";
import { useOverview } from "@/hooks/use-api";

export function EnginePage() {
  const { data: overview } = useOverview();
  const engine = overview?.engine;
  const metrics = engine?.systemMetrics;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Cpu size={22} /> Engine</h1>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Cpu size={16} /> Engine Status</div></div>
        <div style={{ padding: 24 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 24 }}>
            <div><div className="stat-label" style={{ marginBottom: 8 }}>Status</div><StatusBadge status={engine?.status ?? "unknown"} /></div>
            <div><div className="stat-label" style={{ marginBottom: 8 }}>Uptime</div><div className="mono">{engine?.startedAtUnix ? timeAgo(engine.startedAtUnix) : "-"}</div></div>
            <div><div className="stat-label" style={{ marginBottom: 8 }}>CPU Cores</div><div className="mono">{metrics?.cpuCores ?? "-"}</div></div>
          </div>
        </div>
      </div>

      {metrics && (
        <div className="panel">
          <div className="panel-header"><div className="panel-title"><Cpu size={16} /> System Metrics</div></div>
          <div style={{ padding: 24 }}>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 24 }}>
              <MetricBlock label="CPU Usage" value={formatRatio(metrics.cpuUsageRatio ?? 0)} ratio={metrics.cpuUsageRatio ?? 0} />
              <MetricBlock label="Memory" value={`${formatBytes(metrics.memoryUsedBytes ?? "0")} / ${formatBytes(metrics.memoryTotalBytes ?? "0")}`} ratio={parseInt(metrics.memoryUsedBytes ?? "0") / parseInt(metrics.memoryTotalBytes ?? "1")} />
              <MetricBlock label="Disk" value={`${formatBytes(metrics.diskUsedBytes ?? "0")} / ${formatBytes(metrics.diskTotalBytes ?? "0")}`} ratio={parseInt(metrics.diskUsedBytes ?? "0") / parseInt(metrics.diskTotalBytes ?? "1")} />
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function MetricBlock({ label, value, ratio }: { label: string; value: string; ratio: number }) {
  const percent = Math.min((isNaN(ratio) ? 0 : ratio) * 100, 100);
  const color = percent > 80 ? "var(--red)" : percent > 50 ? "var(--yellow)" : "var(--green)";
  return (
    <div>
      <div className="stat-label" style={{ marginBottom: 8 }}>{label}</div>
      <div className="mono" style={{ marginBottom: 8, fontSize: 16, fontWeight: 600 }}>{value}</div>
      <div style={{ height: 4, background: "var(--border)", borderRadius: 2, overflow: "hidden" }}>
        <div style={{ height: "100%", width: `${percent}%`, background: color, borderRadius: 2, transition: "width 0.3s" }} />
      </div>
    </div>
  );
}
