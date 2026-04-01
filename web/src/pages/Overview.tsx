import { useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  RefreshCw,
  Cpu,
  Server,
  Rocket,
  Container,
  ListChecks,
  CheckCircle,
  AlertTriangle,
  XCircle,
  HeartPulse,
  MoreHorizontal,
  Activity,
} from "lucide-react";
import { StatCard } from "@/components/StatCard";
import { StatusBadge } from "@/components/StatusBadge";
import { CPUCell } from "@/components/Sparkline";
import { formatBytes, timeAgo } from "@/lib/format";
import { useOverview, useContainers } from "@/hooks/use-api";
import { useCPUHistory } from "@/hooks/use-cpu-history";

export function Overview() {
  const navigate = useNavigate();
  const { data: overview, loading, error, refresh } = useOverview();
  const { data: containers } = useContainers();
  const cpuHistory = useCPUHistory(containers);

  const summary = overview?.summary;
  const totalTasks = summary?.tasksByStatus
    ? Object.values(summary.tasksByStatus).reduce((a, b) => a + b, 0)
    : 0;
  const failedTasks = summary?.tasksByStatus?.["failed"] ?? 0;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">
          <LayoutDashboard size={22} />
          Cluster Overview
        </h1>
        <div className="header-actions">
          <button className="icon-btn" onClick={refresh} title="Refresh">
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>
      </div>

      {error && (
        <div className="alert alert-error" style={{ marginBottom: 24 }}>
          <XCircle size={16} />
          {error}
        </div>
      )}

      <div className="stat-grid">
        <StatCard
          label="Engines"
          icon={Cpu}
          value={overview?.engine?.status === "running" ? 1 : 0}
          total={1}
          valueColor={overview?.engine?.status === "running" ? "var(--green)" : "var(--red)"}
          subIcon={CheckCircle}
          subText={overview?.engine?.status === "running" ? "healthy" : "down"}
          subColor={overview?.engine?.status === "running" ? "var(--green)" : "var(--red)"}
        />
        <StatCard
          label="Agents"
          icon={Server}
          value={summary?.connectedAgents ?? 0}
          total={summary?.totalAgents ?? 0}
          valueColor={summary && summary.connectedAgents === summary.totalAgents ? "var(--green)" : "var(--yellow)"}
          subIcon={CheckCircle}
          subText={summary && summary.connectedAgents === summary.totalAgents ? "all connected" : `${(summary?.totalAgents ?? 0) - (summary?.connectedAgents ?? 0)} offline`}
          subColor={summary && summary.connectedAgents === summary.totalAgents ? "var(--green)" : "var(--yellow)"}
        />
        <StatCard
          label="Deployments"
          icon={Rocket}
          value={summary?.runningDeployments ?? 0}
          total={summary?.totalDeployments ?? 0}
          valueColor="var(--primary)"
          subIcon={CheckCircle}
          subText="running"
          subColor="var(--green)"
        />
        <StatCard
          label="Containers"
          icon={Container}
          value={summary?.healthyContainers ?? 0}
          total={summary?.totalContainers ?? 0}
          valueColor={summary && summary.healthyContainers === summary.totalContainers ? "var(--green)" : "var(--yellow)"}
          subIcon={HeartPulse}
          subText="healthy"
          subColor="var(--green)"
        />
        <StatCard
          label="Tasks"
          icon={ListChecks}
          value={totalTasks}
          valueColor="var(--text)"
          subIcon={failedTasks > 0 ? AlertTriangle : CheckCircle}
          subText={failedTasks > 0 ? `${failedTasks} failed` : "all ok"}
          subColor={failedTasks > 0 ? "var(--red)" : "var(--green)"}
        />
      </div>

      {/* Containers Table */}
      <div className="panel">
        <div className="panel-header">
          <div className="panel-title">
            <Container size={16} />
            Containers
            <span className="panel-count">{containers?.length ?? 0}</span>
          </div>
        </div>
        {!containers?.length ? (
          <div className="empty-state">
            <Container />
            <h3>No containers</h3>
            <p>Deploy an application to see containers here.</p>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Container</th>
                <th>Service</th>
                <th>Agent</th>
                <th>Status</th>
                <th>CPU</th>
                <th>Memory</th>
                <th>Ports</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {containers.map((c) => (
                <tr key={c.taskId} className="clickable-row" onClick={() => navigate(`/containers/${encodeURIComponent(c.containerName)}`)}>
                  <td className="mono link-text">{c.containerName}</td>
                  <td>{c.serviceName}</td>
                  <td className="mono link-text" onClick={(e) => { e.stopPropagation(); navigate(`/agents/${encodeURIComponent(c.agentId)}`); }}>
                    {c.agentId}
                  </td>
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

      {/* Recent Events */}
      {overview?.recentEvents && overview.recentEvents.length > 0 && (
        <div className="panel">
          <div className="panel-header">
            <div className="panel-title">
              <Activity size={16} />
              Recent Events
              <span className="panel-count">{overview.recentEvents.length}</span>
            </div>
          </div>
          <table>
            <thead>
              <tr>
                <th style={{ width: 100 }}>Time</th>
                <th style={{ width: 100 }}>Severity</th>
                <th style={{ width: 200 }}>Type</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {overview.recentEvents.slice(0, 10).map((ev, i) => (
                <tr key={i}>
                  <td className="mono">{timeAgo(ev.timestampUnix)}</td>
                  <td>
                    <StatusBadge
                      status={ev.severity === "error" ? "failed" : ev.severity === "warning" ? "pending" : "info"}
                      label={ev.severity}
                    />
                  </td>
                  <td className="mono" style={{ fontSize: 12 }}>{ev.type}</td>
                  <td>{ev.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
