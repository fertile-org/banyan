import { Settings, Server, Cpu } from "lucide-react";
import { useOverview, useAgents } from "@/hooks/use-api";
import { formatBytes, formatRatio, timeAgo } from "@/lib/format";
import { StatusBadge } from "@/components/StatusBadge";

export function Configuration() {
  const { data: overview } = useOverview();
  const { data: agents } = useAgents();

  const engine = overview?.engine;
  const summary = overview?.summary;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Settings size={22} /> Configuration</h1>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Cpu size={16} /> Engine</div></div>
        <div style={{ padding: 24 }}>
          {engine ? (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 24 }}>
              <InfoItem label="Status"><StatusBadge status={engine.status} /></InfoItem>
              <InfoItem label="Uptime"><span className="mono">{engine.startedAtUnix ? timeAgo(engine.startedAtUnix) : "-"}</span></InfoItem>
              <InfoItem label="CPU"><span className="mono">{engine.systemMetrics ? formatRatio(engine.systemMetrics.cpuUsageRatio ?? 0) : "-"}</span></InfoItem>
              <InfoItem label="Memory">
                <span className="mono">
                  {engine.systemMetrics ? `${formatBytes(engine.systemMetrics.memoryUsedBytes ?? "0")} / ${formatBytes(engine.systemMetrics.memoryTotalBytes ?? "0")}` : "-"}
                </span>
              </InfoItem>
              <InfoItem label="Disk">
                <span className="mono">
                  {engine.systemMetrics ? `${formatBytes(engine.systemMetrics.diskUsedBytes ?? "0")} / ${formatBytes(engine.systemMetrics.diskTotalBytes ?? "0")}` : "-"}
                </span>
              </InfoItem>
              <InfoItem label="CPU Cores"><span className="mono">{engine.systemMetrics?.cpuCores ?? "-"}</span></InfoItem>
            </div>
          ) : (
            <div className="text-muted">Loading engine info...</div>
          )}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Settings size={16} /> Cluster Summary</div></div>
        <div style={{ padding: 24 }}>
          {summary ? (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 24 }}>
              <InfoItem label="Agents"><span className="mono">{summary.connectedAgents ?? 0} / {summary.totalAgents ?? 0} connected</span></InfoItem>
              <InfoItem label="Deployments"><span className="mono">{summary.runningDeployments ?? 0} / {summary.totalDeployments ?? 0} running</span></InfoItem>
              <InfoItem label="Containers"><span className="mono">{summary.healthyContainers ?? 0} / {summary.totalContainers ?? 0} healthy</span></InfoItem>
              {summary.tasksByStatus && (
                <InfoItem label="Tasks by Status">
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
                    {Object.entries(summary.tasksByStatus).map(([status, count]) => (
                      <span key={status} className="status status-info" style={{ fontSize: 11 }}>{status}: {count}</span>
                    ))}
                  </div>
                </InfoItem>
              )}
            </div>
          ) : (
            <div className="text-muted">Loading cluster summary...</div>
          )}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Server size={16} /> Registered Agents <span className="panel-count">{agents?.length ?? 0}</span></div>
        </div>
        {!agents?.length ? (
          <div className="empty-state"><Server /><h3>No agents</h3><p>Start a banyan-agent to register it with the engine.</p></div>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Status</th><th>Address</th><th>Subnet</th><th>Containers</th><th>Tags</th><th>Last Seen</th></tr></thead>
            <tbody>
              {agents.map((agent) => (
                <tr key={agent.name}>
                  <td className="mono">{agent.name}</td>
                  <td><StatusBadge status={agent.status} /></td>
                  <td className="mono" style={{ fontSize: 11 }}>{agent.apiAddress || "-"}</td>
                  <td className="mono">{agent.vpcSubnet || "-"}</td>
                  <td className="mono">{agent.containerCount ?? 0}</td>
                  <td>{agent.tags?.length ? agent.tags.map((t) => <span key={t} className="status status-info" style={{ marginRight: 4 }}>{t}</span>) : <span className="text-muted">-</span>}</td>
                  <td className="mono">{timeAgo(agent.lastSeenUnix)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}

function InfoItem({ label, children }: { label: string; children: React.ReactNode }) {
  return <div><div className="stat-label" style={{ marginBottom: 6 }}>{label}</div><div>{children}</div></div>;
}
