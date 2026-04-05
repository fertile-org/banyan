import { useNavigate } from "react-router-dom";
import { Rocket, RefreshCw, Trash2 } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { FilterSelect } from "@/components/FilterSelect";
import { timeAgo } from "@/lib/format";
import { useDeployments } from "@/hooks/use-api";
import { useState, useMemo } from "react";

export function Deployments() {
  const navigate = useNavigate();
  const { data: deployments, refresh } = useDeployments();
  const [statusFilter, setStatusFilter] = useState("");

  const statusOptions = useMemo(() => {
    return [...new Set((deployments ?? []).map((d) => d.status))].sort();
  }, [deployments]);

  const filtered = (deployments ?? [])
    .filter((d) => !statusFilter || d.status === statusFilter)
    .sort((a, b) => {
      const aRunning = a.status === "running" ? 0 : 1;
      const bRunning = b.status === "running" ? 0 : 1;
      if (aRunning !== bRunning) return aRunning - bRunning;
      return parseInt(b.createdAtUnix) - parseInt(a.createdAtUnix);
    });

  const totalCount = deployments?.length ?? 0;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Rocket size={22} /> Deployments</h1>
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
          <div className="panel-title"><Rocket size={16} /> All Deployments <span className="panel-count">{filtered.length}</span></div>
        </div>
        {!filtered.length ? (
          <div className="empty-state"><Rocket /><h3>No deployments</h3><p>{statusFilter ? "No deployments match your filter." : <>Use <code className="mono">banyan-cli up</code> to deploy an application.</>}</p></div>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Status</th><th>Health</th><th>Services</th><th>Tags</th><th>Created</th><th>Error</th><th></th></tr></thead>
            <tbody>
              {filtered.map((dep) => {
                const serviceNames = dep.services ? Object.keys(dep.services) : [];
                return (
                  <tr key={dep.id} className="clickable-row" onClick={() => navigate(`/deployments/${encodeURIComponent(dep.id)}`)}>
                    <td className="mono link-text">{dep.name}</td>
                    <td><StatusBadge status={dep.status} /></td>
                    <td className="mono">
                      <span style={{ color: dep.healthy === dep.total ? "var(--green)" : "var(--yellow)" }}>
                        {dep.healthy ?? 0}/{dep.total ?? 0}
                      </span>
                    </td>
                    <td>{serviceNames.map((s) => <span key={s} className="status status-info" style={{ marginRight: 4 }}>{s}</span>)}</td>
                    <td>{dep.tags?.length ? dep.tags.map((t) => <span key={t} className="status status-info" style={{ marginRight: 4 }}>{t}</span>) : <span className="text-muted">-</span>}</td>
                    <td className="mono">{timeAgo(dep.createdAtUnix)}</td>
                    <td style={{ color: "var(--red)", fontSize: 12 }}>{dep.error || ""}</td>
                    <td><button className="btn-ghost" onClick={(e) => e.stopPropagation()} title="Teardown"><Trash2 size={14} /></button></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
