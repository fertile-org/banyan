import { ScrollText } from "lucide-react";
import { useState, useEffect, useRef, useCallback } from "react";
import { useContainers } from "@/hooks/use-api";
import { streamLogs } from "@/api/client";

export function Logs() {
  const { data: containers } = useContainers();
  const containerNames = (containers ?? []).map((c) => c.containerName).sort();

  const [selected, setSelected] = useState<string>("");
  const [lines, setLines] = useState<string[]>([]);
  const [streaming, setStreaming] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);

  const startStream = useCallback(async (containerName: string) => {
    if (abortRef.current) abortRef.current.abort();
    setLines([]);
    setStreaming(true);
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      for await (const chunk of streamLogs(containerName, 100)) {
        if (controller.signal.aborted) break;
        const newLines = chunk.split("\n").filter(Boolean);
        setLines((prev) => [...prev.slice(-500), ...newLines]);
      }
    } catch {
      if (!controller.signal.aborted) setLines((prev) => [...prev, "--- stream ended ---"]);
    } finally {
      setStreaming(false);
    }
  }, []);

  useEffect(() => { return () => { abortRef.current?.abort(); }; }, []);
  useEffect(() => { logEndRef.current?.scrollIntoView({ behavior: "smooth" }); }, [lines]);

  const handleSelect = (name: string) => {
    setSelected(name);
    void startStream(name);
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><ScrollText size={22} /> Logs</h1>
        <div className="header-actions">
          <select className="form-input" value={selected} onChange={(e) => handleSelect(e.target.value)} style={{ maxWidth: 260, fontSize: 12, padding: "5px 10px" }}>
            <option value="">Select a container...</option>
            {containerNames.map((name) => <option key={name} value={name}>{name}</option>)}
          </select>
        </div>
      </div>
      <div className="panel">
        <div className="panel-header">
          <div className="panel-title">
            <ScrollText size={16} /> {selected || "No container selected"}
            {streaming && <span className="status status-running" style={{ fontSize: 10, marginLeft: 8 }}><span className="status-dot" />streaming</span>}
          </div>
          {selected && (
            <div className="panel-actions">
              <button className="btn-ghost" onClick={() => { abortRef.current?.abort(); setStreaming(false); }}>Stop</button>
            </div>
          )}
        </div>
        <div className="log-pane" style={{ minHeight: 300 }}>
          {!selected && <div className="text-muted">Select a container to stream logs.</div>}
          {lines.map((line, i) => <LogLine key={i} line={line} />)}
          <div ref={logEndRef} />
        </div>
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
