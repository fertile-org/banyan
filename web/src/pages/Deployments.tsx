import { useNavigate } from "react-router-dom";
import { Rocket, RefreshCw, Trash2 } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { timeAgo } from "@/lib/format";
import { useDeployments } from "@/hooks/use-api";

export function Deployments() {
  const navigate = useNavigate();
  const { data: deployments, refresh } = useDeployments();

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Rocket size={22} /> Deployments</h1>
        <div className="header-actions">
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>
      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Rocket size={16} /> All Deployments <span className="panel-count">{deployments?.length ?? 0}</span></div>
        </div>
        {!deployments?.length ? (
          <div className="empty-state"><Rocket /><h3>No deployments</h3><p>Use <code className="mono">banyan-cli up</code> to deploy an application.</p></div>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Status</th><th>Health</th><th>Services</th><th>Tags</th><th>Created</th><th>Error</th><th></th></tr></thead>
            <tbody>
              {deployments.map((dep) => {
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
