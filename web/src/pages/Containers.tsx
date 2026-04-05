import { useNavigate } from "react-router-dom";
import { Container, RefreshCw, MoreHorizontal } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { FilterSelect } from "@/components/FilterSelect";
import { CPUCell } from "@/components/Sparkline";
import { formatBytes, timeAgo } from "@/lib/format";
import { useContainers } from "@/hooks/use-api";
import { useCPUHistory } from "@/hooks/use-cpu-history";
import { useState, useMemo } from "react";

export function Containers() {
  const navigate = useNavigate();
  const { data: allContainers, refresh } = useContainers();
  const cpuHistory = useCPUHistory(allContainers);
  const [filter, setFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [agentFilter, setAgentFilter] = useState("");
  const [deploymentFilter, setDeploymentFilter] = useState("");
  const [serviceFilter, setServiceFilter] = useState("");

  const filterOptions = useMemo(() => {
    const all = allContainers ?? [];
    return {
      statuses: [...new Set(all.map((c) => c.containerStatus))].sort(),
      agents: [...new Set(all.map((c) => c.agentId))].sort(),
      deployments: [...new Set(all.map((c) => c.deploymentName))].sort(),
      services: [...new Set(all.map((c) => c.serviceName))].sort(),
    };
  }, [allContainers]);

  const containers = (allContainers ?? []).filter((c) => {
    if (statusFilter && c.containerStatus !== statusFilter) return false;
    if (agentFilter && c.agentId !== agentFilter) return false;
    if (deploymentFilter && c.deploymentName !== deploymentFilter) return false;
    if (serviceFilter && c.serviceName !== serviceFilter) return false;
    if (filter) {
      const lower = filter.toLowerCase();
      return (
        c.containerName.toLowerCase().includes(lower) ||
        c.serviceName.toLowerCase().includes(lower) ||
        c.agentId.toLowerCase().includes(lower) ||
        c.deploymentName.toLowerCase().includes(lower)
      );
    }
    return true;
  }).sort((a, b) => {
    const aRunning = a.containerStatus === "running" ? 0 : 1;
    const bRunning = b.containerStatus === "running" ? 0 : 1;
    if (aRunning !== bRunning) return aRunning - bRunning;
    return parseInt(b.createdAtUnix) - parseInt(a.createdAtUnix);
  });

  const hasFilters = !!(statusFilter || agentFilter || deploymentFilter || serviceFilter || filter);
  const totalCount = allContainers?.length ?? 0;

  const clearFilters = () => {
    setFilter("");
    setStatusFilter("");
    setAgentFilter("");
    setDeploymentFilter("");
    setServiceFilter("");
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Container size={22} /> Containers</h1>
        <div className="header-actions">
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>

      <div className="filter-bar">
        <FilterSelect value={statusFilter} onChange={setStatusFilter} options={filterOptions.statuses} placeholder="All statuses" width={130} />
        <FilterSelect value={agentFilter} onChange={setAgentFilter} options={filterOptions.agents} placeholder="All agents" width={140} />
        <FilterSelect value={deploymentFilter} onChange={setDeploymentFilter} options={filterOptions.deployments} placeholder="All deployments" width={150} />
        <FilterSelect value={serviceFilter} onChange={setServiceFilter} options={filterOptions.services} placeholder="All services" width={140} />
        <input className="form-input" placeholder="Search..." value={filter} onChange={(e) => setFilter(e.target.value)} />
        {hasFilters && (
          <>
            <span className="filter-info">Showing {containers.length} of {totalCount}</span>
            <button className="btn btn-secondary" onClick={clearFilters} style={{ fontSize: 12, padding: "4px 10px" }}>Clear</button>
          </>
        )}
      </div>

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Container size={16} /> All Containers <span className="panel-count">{containers.length}</span></div>
        </div>
        {!containers.length ? (
          <div className="empty-state"><Container /><h3>No containers</h3><p>{hasFilters ? "No containers match your filters." : "Deploy an application to see containers here."}</p></div>
        ) : (
          <table>
            <thead><tr><th>Container</th><th>Service</th><th>Deployment</th><th>Agent</th><th>Status</th><th>Health</th><th>CPU</th><th>Memory</th><th>Ports</th><th></th></tr></thead>
            <tbody>
              {containers.map((c) => (
                <tr key={c.taskId} className="clickable-row" onClick={() => navigate(`/containers/${encodeURIComponent(c.containerName)}`)}>
                  <td className="mono link-text">{c.containerName}</td>
                  <td>{c.serviceName}</td>
                  <td className="mono link-text" onClick={(e) => { e.stopPropagation(); navigate(`/deployments/${encodeURIComponent(c.deploymentId)}`); }} title={c.deploymentId}>{c.deploymentName} <span className="text-muted" style={{ fontSize: 11 }}>({timeAgo(c.createdAtUnix)})</span></td>
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
