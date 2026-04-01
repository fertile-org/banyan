import { useParams, useNavigate, Link } from "react-router-dom";
import { Rocket, ArrowLeft, Container, MoreHorizontal, Scaling } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { CPUCell } from "@/components/Sparkline";
import { formatBytes, timeAgo } from "@/lib/format";
import { useDeploymentDetail, useContainers } from "@/hooks/use-api";
import { useCPUHistory } from "@/hooks/use-cpu-history";

export function DeploymentDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: deployment } = useDeploymentDetail(id ?? "");
  const { data: allContainers } = useContainers();
  const cpuHistory = useCPUHistory(allContainers);

  const containers = allContainers?.filter((c) => c.deploymentId === id) ?? [];

  if (!deployment) {
    return (
      <div className="empty-state"><Rocket /><h3>Deployment not found</h3>
        <Link to="/deployments" className="btn btn-secondary" style={{ marginTop: 16 }}><ArrowLeft size={14} /> Back to Deployments</Link>
      </div>
    );
  }

  const services = deployment.services ? Object.entries(deployment.services) : [];

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">
          <button className="btn-ghost" onClick={() => navigate("/deployments")} style={{ marginRight: 4 }}><ArrowLeft size={18} /></button>
          <Rocket size={22} /> {deployment.name} <StatusBadge status={deployment.status} />
        </h1>
        <div className="header-actions">
          <span className="mono text-muted" style={{ fontSize: 11 }}>{deployment.id}</span>
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Rocket size={16} /> Deployment Details</div></div>
        <div style={{ padding: 24 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 24 }}>
            <InfoItem label="Status"><StatusBadge status={deployment.status} /></InfoItem>
            <InfoItem label="Health">
              <span className="mono" style={{ fontSize: 16, fontWeight: 600, color: deployment.healthy === deployment.total ? "var(--green)" : "var(--yellow)" }}>
                {deployment.healthy ?? 0}/{deployment.total ?? 0}
              </span>
            </InfoItem>
            <InfoItem label="Created"><span className="mono">{timeAgo(deployment.createdAtUnix)}</span></InfoItem>
            <InfoItem label="Tags">{deployment.tags?.length ? deployment.tags.map((t) => <span key={t} className="status status-info" style={{ marginRight: 4 }}>{t}</span>) : <span className="text-muted">-</span>}</InfoItem>
          </div>
          {deployment.error && <div className="alert alert-error" style={{ marginTop: 16 }}>{deployment.error}</div>}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Scaling size={16} /> Services <span className="panel-count">{services.length}</span></div></div>
        {services.length > 0 && (
          <table>
            <thead><tr><th>Service</th><th>Image</th><th>Replicas</th><th>Ports</th><th>Dependencies</th></tr></thead>
            <tbody>
              {services.map(([name, svc]) => (
                <tr key={name}>
                  <td className="mono" style={{ fontWeight: 600 }}>{name}</td>
                  <td className="mono">{svc.image}</td>
                  <td className="mono">{svc.replicas}</td>
                  <td className="mono">{svc.ports?.join(", ") || "-"}</td>
                  <td className="mono">{svc.dependsOn ? Object.keys(svc.dependsOn).join(", ") : "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Container size={16} /> Containers <span className="panel-count">{containers.length}</span></div></div>
        {!containers.length ? (
          <div className="empty-state"><Container /><h3>No containers</h3></div>
        ) : (
          <table>
            <thead><tr><th>Container</th><th>Service</th><th>Agent</th><th>Status</th><th>CPU</th><th>Memory</th><th>Ports</th><th></th></tr></thead>
            <tbody>
              {containers.map((c) => (
                <tr key={c.taskId} className="clickable-row" onClick={() => navigate(`/containers/${encodeURIComponent(c.containerName)}`)}>
                  <td className="mono link-text">{c.containerName}</td>
                  <td>{c.serviceName}</td>
                  <td className="mono link-text" onClick={(e) => { e.stopPropagation(); navigate(`/agents/${encodeURIComponent(c.agentId)}`); }}>{c.agentId}</td>
                  <td><StatusBadge status={c.containerStatus} /></td>
                  <td><CPUCell percent={c.cpuPercent ?? 0} history={cpuHistory[c.containerName] ?? []} /></td>
                  <td className="mono">{formatBytes(c.memoryUsedBytes ?? "0")}</td>
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

function InfoItem({ label, children }: { label: string; children: React.ReactNode }) {
  return <div><div className="stat-label" style={{ marginBottom: 6 }}>{label}</div><div>{children}</div></div>;
}
