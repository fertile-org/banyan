import { useNavigate } from "react-router-dom";
import { Container, RefreshCw, MoreHorizontal } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { CPUCell } from "@/components/Sparkline";
import { formatBytes } from "@/lib/format";
import { useContainers } from "@/hooks/use-api";
import { useCPUHistory } from "@/hooks/use-cpu-history";
import { useState } from "react";

export function Containers() {
  const navigate = useNavigate();
  const { data: allContainers, refresh } = useContainers();
  const cpuHistory = useCPUHistory(allContainers);
  const [filter, setFilter] = useState("");

  const containers = (allContainers ?? []).filter((c) => {
    if (!filter) return true;
    const lower = filter.toLowerCase();
    return (
      c.containerName.toLowerCase().includes(lower) ||
      c.serviceName.toLowerCase().includes(lower) ||
      c.agentId.toLowerCase().includes(lower) ||
      c.deploymentName.toLowerCase().includes(lower)
    );
  });

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Container size={22} /> Containers</h1>
        <div className="header-actions">
          <input className="form-input" placeholder="Filter containers..." value={filter} onChange={(e) => setFilter(e.target.value)} style={{ maxWidth: 220, fontSize: 12, padding: "5px 10px" }} />
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>
      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Container size={16} /> All Containers <span className="panel-count">{containers.length}</span></div>
        </div>
        {!containers.length ? (
          <div className="empty-state"><Container /><h3>No containers</h3><p>{filter ? "No containers match your filter." : "Deploy an application to see containers here."}</p></div>
        ) : (
          <table>
            <thead><tr><th>Container</th><th>Service</th><th>Deployment</th><th>Agent</th><th>Status</th><th>Health</th><th>CPU</th><th>Memory</th><th>Ports</th><th></th></tr></thead>
            <tbody>
              {containers.map((c) => (
                <tr key={c.taskId} className="clickable-row" onClick={() => navigate(`/containers/${encodeURIComponent(c.containerName)}`)}>
                  <td className="mono link-text">{c.containerName}</td>
                  <td>{c.serviceName}</td>
                  <td className="mono link-text" onClick={(e) => { e.stopPropagation(); navigate(`/deployments/${encodeURIComponent(c.deploymentId)}`); }}>{c.deploymentName}</td>
                  <td className="mono link-text" onClick={(e) => { e.stopPropagation(); navigate(`/agents/${encodeURIComponent(c.agentId)}`); }}>{c.agentId}</td>
                  <td><StatusBadge status={c.containerStatus} /></td>
                  <td>{c.healthStatus ? <StatusBadge status={c.healthStatus} /> : <span className="text-muted">-</span>}</td>
                  <td><CPUCell percent={c.cpuPercent ?? 0} history={cpuHistory[c.containerName] ?? []} /></td>
                  <td className="mono">{c.memoryUsedBytes && c.memoryLimitBytes ? `${formatBytes(c.memoryUsedBytes)} / ${formatBytes(c.memoryLimitBytes)}` : formatBytes(c.memoryUsedBytes ?? "0")}</td>
                  <td className="mono">{c.ports?.join(", ") || "-"}</td>
                  <td><button className="btn-ghost" onClick={(e) => e.stopPropagation()}><MoreHorizontal size={14} /></button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
