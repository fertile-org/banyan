import { useParams, useNavigate, Link } from "react-router-dom";
import { Server, ArrowLeft, Container, MoreHorizontal } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { CPUCell } from "@/components/Sparkline";
import { formatBytes, formatRatio, timeAgo } from "@/lib/format";
import { useAgents, useContainers } from "@/hooks/use-api";
import { useCPUHistory } from "@/hooks/use-cpu-history";

export function AgentDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: agents } = useAgents();
  const { data: allContainers } = useContainers();
  const cpuHistory = useCPUHistory(allContainers);

  const agent = agents?.find((a) => a.name === name);
  const containers = allContainers?.filter((c) => c.agentId === name) ?? [];

  if (!agent) {
    return (
      <div className="empty-state"><Server /><h3>Agent not found</h3><p>Agent "{name}" does not exist.</p>
        <Link to="/agents" className="btn btn-secondary" style={{ marginTop: 16 }}><ArrowLeft size={14} /> Back to Agents</Link>
      </div>
    );
  }

  const metrics = agent.systemMetrics;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">
          <button className="btn-ghost" onClick={() => navigate("/agents")} style={{ marginRight: 4 }}><ArrowLeft size={18} /></button>
          <Server size={22} /> {agent.name} <StatusBadge status={agent.status} />
        </h1>
      </div>
      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Server size={16} /> Agent Details</div></div>
        <div style={{ padding: 24 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 24 }}>
            <InfoItem label="Status"><StatusBadge status={agent.status} /></InfoItem>
            <InfoItem label="API Address"><span className="mono">{agent.apiAddress || "-"}</span></InfoItem>
            <InfoItem label="VPC Subnet"><span className="mono">{agent.vpcSubnet || "-"}</span></InfoItem>
            <InfoItem label="Last Seen"><span className="mono">{timeAgo(agent.lastSeenUnix)}</span></InfoItem>
            <InfoItem label="Containers"><span className="mono">{agent.containerCount ?? 0}</span></InfoItem>
            <InfoItem label="CPU Cores"><span className="mono">{metrics?.cpuCores ?? "-"}</span></InfoItem>
            <InfoItem label="Tags">{agent.tags?.length ? agent.tags.map((t) => <span key={t} className="status status-info" style={{ marginRight: 4 }}>{t}</span>) : <span className="text-muted">-</span>}</InfoItem>
            <InfoItem label="Registered"><span className="mono">{timeAgo(agent.createdAtUnix)}</span></InfoItem>
          </div>
        </div>
      </div>
      {metrics && (
        <div className="panel">
          <div className="panel-header"><div className="panel-title">System Metrics</div></div>
          <div style={{ padding: 24 }}>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 24 }}>
              <MetricBar label="CPU" value={formatRatio(metrics.cpuUsageRatio ?? 0)} ratio={metrics.cpuUsageRatio ?? 0} />
              <MetricBar label="Memory" value={`${formatBytes(metrics.memoryUsedBytes ?? "0")} / ${formatBytes(metrics.memoryTotalBytes ?? "0")}`} ratio={parseInt(metrics.memoryUsedBytes ?? "0") / parseInt(metrics.memoryTotalBytes ?? "1")} />
              <MetricBar label="Disk" value={`${formatBytes(metrics.diskUsedBytes ?? "0")} / ${formatBytes(metrics.diskTotalBytes ?? "0")}`} ratio={parseInt(metrics.diskUsedBytes ?? "0") / parseInt(metrics.diskTotalBytes ?? "1")} />
            </div>
          </div>
        </div>
      )}
      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Container size={16} /> Containers on {agent.name} <span className="panel-count">{containers.length}</span></div></div>
        {!containers.length ? (
          <div className="empty-state"><Container /><h3>No containers</h3><p>This agent has no running containers.</p></div>
        ) : (
          <table>
            <thead><tr><th>Container</th><th>Service</th><th>Deployment</th><th>Status</th><th>CPU</th><th>Memory</th><th>Ports</th><th></th></tr></thead>
            <tbody>
              {containers.map((c) => (
                <tr key={c.taskId} className="clickable-row" onClick={() => navigate(`/containers/${encodeURIComponent(c.containerName)}`)}>
                  <td className="mono link-text">{c.containerName}</td>
                  <td>{c.serviceName}</td>
                  <td className="mono link-text" onClick={(e) => { e.stopPropagation(); navigate(`/deployments/${encodeURIComponent(c.deploymentId)}`); }}>{c.deploymentName}</td>
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

function MetricBar({ label, value, ratio }: { label: string; value: string; ratio: number }) {
  const percent = Math.min((isNaN(ratio) ? 0 : ratio) * 100, 100);
  const color = percent > 80 ? "var(--red)" : percent > 50 ? "var(--yellow)" : "var(--green)";
  return (
    <div>
      <div className="stat-label" style={{ marginBottom: 8 }}>{label}</div>
      <div className="mono" style={{ marginBottom: 8, fontSize: 14, fontWeight: 600 }}>{value}</div>
      <div style={{ height: 4, background: "var(--border)", borderRadius: 2, overflow: "hidden" }}>
        <div style={{ height: "100%", width: `${percent}%`, background: color, borderRadius: 2, transition: "width 0.3s" }} />
      </div>
    </div>
  );
}
