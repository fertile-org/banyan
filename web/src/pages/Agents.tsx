import { useNavigate } from "react-router-dom";
import { Server, RefreshCw } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { formatBytes, formatRatio, timeAgo } from "@/lib/format";
import { useAgents } from "@/hooks/use-api";

export function Agents() {
  const navigate = useNavigate();
  const { data: agents, refresh } = useAgents();

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Server size={22} /> Agents</h1>
        <div className="header-actions">
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>
      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Server size={16} /> All Agents <span className="panel-count">{agents?.length ?? 0}</span></div>
        </div>
        {!agents?.length ? (
          <div className="empty-state"><Server /><h3>No agents registered</h3><p>Start a banyan-agent to see it here.</p></div>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Status</th><th>Containers</th><th>CPU</th><th>Memory</th><th>Disk</th><th>Subnet</th><th>Tags</th><th>Last Seen</th></tr></thead>
            <tbody>
              {agents.map((agent) => (
                <tr key={agent.name} className="clickable-row" onClick={() => navigate(`/agents/${encodeURIComponent(agent.name)}`)}>
                  <td className="mono link-text">{agent.name}</td>
                  <td><StatusBadge status={agent.status} /></td>
                  <td className="mono">{agent.containerCount ?? 0}</td>
                  <td className="mono">{agent.systemMetrics ? formatRatio(agent.systemMetrics.cpuUsageRatio ?? 0) : "-"}</td>
                  <td className="mono">{agent.systemMetrics ? `${formatBytes(agent.systemMetrics.memoryUsedBytes ?? "0")} / ${formatBytes(agent.systemMetrics.memoryTotalBytes ?? "0")}` : "-"}</td>
                  <td className="mono">{agent.systemMetrics ? `${formatBytes(agent.systemMetrics.diskUsedBytes ?? "0")} / ${formatBytes(agent.systemMetrics.diskTotalBytes ?? "0")}` : "-"}</td>
                  <td className="mono">{agent.vpcSubnet || "-"}</td>
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
