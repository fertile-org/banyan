import { ScrollText, RefreshCw, Pause, Play } from "lucide-react";
import { useState, useEffect, useRef, useCallback } from "react";
import { useContainers } from "@/hooks/use-api";
import { getRecentLogs } from "@/api/client";

const TAIL_OPTIONS = [100, 500, 1000] as const;
const POLL_INTERVAL = 3000;

export function Logs() {
  const { data: containers } = useContainers();
  const containerNames = (containers ?? []).map((c) => c.containerName).sort();

  const [selected, setSelected] = useState("");
  const [lines, setLines] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tail, setTail] = useState<number>(500);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [userScrolledUp, setUserScrolledUp] = useState(false);

  const logPaneRef = useRef<HTMLDivElement>(null);
  const logEndRef = useRef<HTMLDivElement>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchLogs = useCallback(async (containerName: string, tailCount: number) => {
    try {
      const resp = await getRecentLogs(containerName, tailCount);
      setLines(resp.lines ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch logs");
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial fetch when container is selected
  const handleSelect = (name: string) => {
    setSelected(name);
    setLines([]);
    setError(null);
    setUserScrolledUp(false);
    if (name) {
      setLoading(true);
      void fetchLogs(name, tail);
    }
  };

  // Auto-refresh polling
  useEffect(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    if (!selected || !autoRefresh) return;

    intervalRef.current = setInterval(() => void fetchLogs(selected, tail), POLL_INTERVAL);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [selected, autoRefresh, tail, fetchLogs]);

  // Auto-scroll to bottom (unless user scrolled up)
  useEffect(() => {
    if (!userScrolledUp && logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [lines, userScrolledUp]);

  // Detect if user scrolled up from bottom
  const handleScroll = () => {
    const pane = logPaneRef.current;
    if (!pane) return;
    const atBottom = pane.scrollHeight - pane.scrollTop - pane.clientHeight < 40;
    setUserScrolledUp(!atBottom);
  };

  // Re-fetch when tail changes
  const handleTailChange = (newTail: number) => {
    setTail(newTail);
    if (selected) {
      setLoading(true);
      void fetchLogs(selected, newTail);
    }
  };

  const handleRefresh = () => {
    if (selected) {
      setLoading(true);
      void fetchLogs(selected, tail);
    }
  };

  const scrollToBottom = () => {
    setUserScrolledUp(false);
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><ScrollText size={22} /> Logs</h1>
        <div className="header-actions">
          <select
            className="form-input"
            value={selected}
            onChange={(e) => handleSelect(e.target.value)}
            style={{ maxWidth: 260, fontSize: 12, padding: "5px 10px" }}
          >
            <option value="">Select a container...</option>
            {containerNames.map((name) => (
              <option key={name} value={name}>{name}</option>
            ))}
          </select>
          <select
            className="form-input"
            value={tail}
            onChange={(e) => handleTailChange(Number(e.target.value))}
            style={{ maxWidth: 100, fontSize: 12, padding: "5px 10px" }}
          >
            {TAIL_OPTIONS.map((n) => (
              <option key={n} value={n}>{n} lines</option>
            ))}
          </select>
          <button
            className={`icon-btn${autoRefresh ? " active-toggle" : ""}`}
            onClick={() => setAutoRefresh((v) => !v)}
            title={autoRefresh ? "Pause auto-refresh" : "Resume auto-refresh"}
          >
            {autoRefresh ? <Pause size={14} /> : <Play size={14} />}
          </button>
          <button className="icon-btn" onClick={handleRefresh} title="Refresh now">
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
          </button>
        </div>
      </div>

      <div className="panel" style={{ position: "relative" }}>
        <div className="panel-header">
          <div className="panel-title">
            <ScrollText size={16} />
            {selected || "No container selected"}
            {selected && autoRefresh && (
              <span className="status status-running" style={{ fontSize: 10, marginLeft: 8 }}>
                <span className="status-dot" />
                live
              </span>
            )}
          </div>
          {selected && (
            <div className="panel-actions">
              <span className="mono text-muted" style={{ fontSize: 11 }}>
                {lines.length} lines
              </span>
            </div>
          )}
        </div>

        <div
          ref={logPaneRef}
          className="log-pane"
          style={{ minHeight: 400, maxHeight: "calc(100vh - 240px)" }}
          onScroll={handleScroll}
        >
          {!selected && (
            <div className="text-muted">Select a container to view logs.</div>
          )}
          {error && (
            <div style={{ color: "var(--red)" }}>Error: {error}</div>
          )}
          {selected && !loading && lines.length === 0 && !error && (
            <div className="text-muted">No log output.</div>
          )}
          {lines.map((line, i) => (
            <LogLine key={i} line={line} />
          ))}
          <div ref={logEndRef} />
        </div>

        {/* "New logs" indicator when user scrolled up */}
        {userScrolledUp && autoRefresh && (
          <button
            onClick={scrollToBottom}
            style={{
              position: "absolute",
              bottom: 16,
              right: 16,
              background: "var(--primary)",
              color: "#fff",
              border: "none",
              borderRadius: "var(--radius-md)",
              padding: "6px 12px",
              fontSize: 12,
              fontWeight: 500,
              cursor: "pointer",
              boxShadow: "0 2px 8px rgba(0,0,0,0.3)",
            }}
          >
            ↓ Scroll to latest
          </button>
        )}
      </div>
    </>
  );
}

function LogLine({ line }: { line: string }) {
  const hasError = /\bERROR\b/i.test(line);
  const hasWarn = /\bWARN(ING)?\b/i.test(line);
  const hasInfo = /\bINFO\b/i.test(line);
  let cls = "log-msg";
  if (hasError) cls = "log-level-error";
  else if (hasWarn) cls = "log-level-warn";
  else if (hasInfo) cls = "log-level-info";
  return <div><span className={cls}>{line}</span></div>;
}
