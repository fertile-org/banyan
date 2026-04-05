import { useParams, useNavigate, Link } from "react-router-dom";
import { Container, ArrowLeft, ScrollText, Square, ChevronDown, ChevronRight, RefreshCw } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { Sparkline } from "@/components/Sparkline";
import { formatBytes, formatCPU, timeAgo } from "@/lib/format";
import { useContainers } from "@/hooks/use-api";
import { useCPUHistory } from "@/hooks/use-cpu-history";
import { stopTask, getRecentLogs } from "@/api/client";
import { useState, useEffect, useCallback } from "react";

export function ContainerDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { data: allContainers, refresh } = useContainers();
  const cpuHistory = useCPUHistory(allContainers);
  const [stopping, setStopping] = useState(false);

  const container = allContainers?.find((c) => c.containerName === name);

  if (!container) {
    return (
      <div className="empty-state"><Container /><h3>Container not found</h3><p>Container "{name}" does not exist.</p>
        <Link to="/containers" className="btn btn-secondary" style={{ marginTop: 16 }}><ArrowLeft size={14} /> Back to Containers</Link>
      </div>
    );
  }

  const handleStop = async () => {
    if (!confirm(`Stop container ${container.containerName}?`)) return;
    setStopping(true);
    try {
      await stopTask({ taskId: container.taskId, agentId: container.agentId });
      refresh();
    } catch (err) {
      alert(`Failed to stop: ${err instanceof Error ? err.message : "unknown error"}`);
    } finally {
      setStopping(false);
    }
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">
          <button className="btn-ghost" onClick={() => navigate("/containers")} style={{ marginRight: 4 }}><ArrowLeft size={18} /></button>
          <Container size={22} /> {container.containerName} <StatusBadge status={container.containerStatus} />
        </h1>
        <div className="header-actions">
          <button className="btn btn-secondary" onClick={() => navigate(`/logs?container=${encodeURIComponent(container.containerName)}&agent=${encodeURIComponent(container.agentId)}`)}><ScrollText size={14} /> Logs</button>
          <button className="btn btn-danger" onClick={() => void handleStop()} disabled={stopping}><Square size={14} /> {stopping ? "Stopping..." : "Stop"}</button>
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title"><Container size={16} /> Container Details</div></div>
        <div style={{ padding: 24 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 24 }}>
            <InfoItem label="Container Name"><span className="mono">{container.containerName}</span></InfoItem>
            <InfoItem label="Status"><StatusBadge status={container.containerStatus} /></InfoItem>
            <InfoItem label="Health">{container.healthStatus ? <StatusBadge status={container.healthStatus} /> : <span className="text-muted">-</span>}</InfoItem>
            <InfoItem label="Image"><span className="mono" style={{ fontSize: 11, wordBreak: "break-all" }}>{container.image}</span></InfoItem>
            <InfoItem label="Service"><span className="mono">{container.serviceName}</span></InfoItem>
            <InfoItem label="Deployment"><span className="mono link-text" onClick={() => navigate(`/deployments/${encodeURIComponent(container.deploymentId)}`)} style={{ cursor: "pointer" }}>{container.deploymentName}</span></InfoItem>
            <InfoItem label="Agent"><span className="mono link-text" onClick={() => navigate(`/agents/${encodeURIComponent(container.agentId)}`)} style={{ cursor: "pointer" }}>{container.agentId}</span></InfoItem>
            <InfoItem label="Replica"><span className="mono">{container.replicaIndex ?? 0}</span></InfoItem>
            <InfoItem label="Ports"><span className="mono">{container.ports?.join(", ") || "-"}</span></InfoItem>
            <InfoItem label="Created"><span className="mono">{timeAgo(container.createdAtUnix)}</span></InfoItem>
            <InfoItem label="Task ID"><span className="mono" style={{ fontSize: 10 }}>{container.taskId}</span></InfoItem>
          </div>
          {container.error && <div className="alert alert-error" style={{ marginTop: 16 }}>{container.error}</div>}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><div className="panel-title">Resource Usage</div></div>
        <div style={{ padding: 24 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 24 }}>
            <div>
              <div className="stat-label" style={{ marginBottom: 8 }}>CPU</div>
              <div className="mono" style={{ fontSize: 20, fontWeight: 600, marginBottom: 8 }}>{formatCPU(container.cpuPercent ?? 0)}</div>
              <Sparkline values={cpuHistory[container.containerName] ?? []} />
            </div>
            <div>
              <div className="stat-label" style={{ marginBottom: 8 }}>Memory Used</div>
              <div className="mono" style={{ fontSize: 20, fontWeight: 600 }}>{formatBytes(container.memoryUsedBytes ?? "0")}</div>
              {container.memoryLimitBytes && container.memoryLimitBytes !== "0" && (
                <div className="text-muted" style={{ fontSize: 11, marginTop: 4 }}>Limit: {formatBytes(container.memoryLimitBytes)}</div>
              )}
            </div>
            <div>
              <div className="stat-label" style={{ marginBottom: 8 }}>Memory Utilization</div>
              {container.memoryLimitBytes && container.memoryLimitBytes !== "0" ? (
                <MemoryBar used={container.memoryUsedBytes ?? "0"} limit={container.memoryLimitBytes} />
              ) : (
                <span className="text-muted">No limit set</span>
              )}
            </div>
          </div>
        </div>
      </div>

      <InlineLogPanel containerName={container.containerName} agentId={container.agentId} />
    </>
  );
}

function InlineLogPanel({ containerName, agentId }: { containerName: string; agentId: string }) {
  const [expanded, setExpanded] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await getRecentLogs(containerName, 50, agentId);
      setLines(resp.lines ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch logs");
    } finally {
      setLoading(false);
    }
  }, [containerName, agentId]);

  useEffect(() => {
    if (expanded && lines.length === 0 && !error) {
      void fetchLogs();
    }
  }, [expanded]);

  return (
    <div className="panel">
      <div className="panel-header" style={{ cursor: "pointer" }} onClick={() => setExpanded((v) => !v)}>
        <div className="panel-title">
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <ScrollText size={16} /> Recent Logs
        </div>
        {expanded && (
          <div className="panel-actions">
            <button className="icon-btn" onClick={(e) => { e.stopPropagation(); void fetchLogs(); }} title="Refresh logs">
              <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            </button>
          </div>
        )}
      </div>
      <div className={`log-panel-collapse${expanded ? " expanded" : ""}`}>
        <div>
          <div className="log-pane" style={{ maxHeight: 300 }}>
            {loading && lines.length === 0 && <div className="text-muted">Loading logs...</div>}
            {error && <div style={{ color: "var(--red)" }}>Error: {error}</div>}
            {!loading && lines.length === 0 && !error && <div className="text-muted">No log output.</div>}
            {lines.map((line, i) => {
              const hasError = /\bERROR\b/i.test(line);
              const hasWarn = /\bWARN(ING)?\b/i.test(line);
              const hasInfo = /\bINFO\b/i.test(line);
              let cls = "log-msg";
              if (hasError) cls = "log-level-error";
              else if (hasWarn) cls = "log-level-warn";
              else if (hasInfo) cls = "log-level-info";
              return <div key={i}><span className={cls}>{line}</span></div>;
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

function InfoItem({ label, children }: { label: string; children: React.ReactNode }) {
  return <div><div className="stat-label" style={{ marginBottom: 6 }}>{label}</div><div>{children}</div></div>;
}

function MemoryBar({ used, limit }: { used: string; limit: string }) {
  const u = parseInt(used) || 0;
  const l = parseInt(limit) || 1;
  const percent = Math.min((u / l) * 100, 100);
  const color = percent > 80 ? "var(--red)" : percent > 50 ? "var(--yellow)" : "var(--green)";
  return (
    <div>
      <div className="mono" style={{ fontSize: 20, fontWeight: 600, marginBottom: 8 }}>{percent.toFixed(1)}%</div>
      <div style={{ height: 4, background: "var(--border)", borderRadius: 2, overflow: "hidden" }}>
        <div style={{ height: "100%", width: `${percent}%`, background: color, borderRadius: 2, transition: "width 0.3s" }} />
      </div>
    </div>
  );
}
