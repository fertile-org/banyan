import { useNavigate } from "react-router-dom";
import { Server, RefreshCw } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { FilterSelect } from "@/components/FilterSelect";
import { formatBytes, formatRatio, timeAgo } from "@/lib/format";
import { useAgents } from "@/hooks/use-api";
import { useState, useMemo } from "react";

export function Agents() {
  const navigate = useNavigate();
  const { data: agents, refresh } = useAgents();
  const [statusFilter, setStatusFilter] = useState("");

  const statusOptions = useMemo(() => {
    return [...new Set((agents ?? []).map((a) => a.status))].sort();
  }, [agents]);

  const filtered = (agents ?? [])
    .filter((a) => !statusFilter || a.status === statusFilter)
    .sort((a, b) => {
      const aReady = a.status === "ready" ? 0 : 1;
      const bReady = b.status === "ready" ? 0 : 1;
      if (aReady !== bReady) return aReady - bReady;
      return a.name.localeCompare(b.name);
    });

  const totalCount = agents?.length ?? 0;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Server size={22} /> Agents</h1>
        <div className="header-actions">
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>

      <div className="filter-bar">
        <FilterSelect value={statusFilter} onChange={setStatusFilter} options={statusOptions} placeholder="All statuses" width={140} />
        {statusFilter && (
          <>
            <span className="filter-info">Showing {filtered.length} of {totalCount}</span>
            <button className="btn btn-secondary" onClick={() => setStatusFilter("")} style={{ fontSize: 12, padding: "4px 10px" }}>Clear</button>
          </>
        )}
      </div>

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Server size={16} /> All Agents <span className="panel-count">{filtered.length}</span></div>
        </div>
        {!filtered.length ? (
          <div className="empty-state"><Server /><h3>No agents registered</h3><p>{statusFilter ? "No agents match your filter." : "Start a banyan-agent to see it here."}</p></div>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Status</th><th>Containers</th><th>CPU</th><th>Memory</th><th>Disk</th><th>Subnet</th><th>Tags</th><th>Last Seen</th></tr></thead>
            <tbody>
              {filtered.map((agent) => (
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
