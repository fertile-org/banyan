import { Activity } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { timeAgo, formatTime } from "@/lib/format";
import { useEvents } from "@/hooks/use-api";

export function Events() {
  const { data: events } = useEvents();

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Activity size={22} /> Events</h1>
      </div>
      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Activity size={16} /> Recent Events <span className="panel-count">{events?.length ?? 0}</span></div>
        </div>
        {!events?.length ? (
          <div className="empty-state"><Activity /><h3>No events</h3><p>Events will appear here as the cluster operates.</p></div>
        ) : (
          <table>
            <thead><tr><th style={{ width: 80 }}>Time</th><th style={{ width: 80 }}>Ago</th><th style={{ width: 80 }}>Severity</th><th style={{ width: 200 }}>Type</th><th>Message</th></tr></thead>
            <tbody>
              {events.map((ev, i) => (
                <tr key={i}>
                  <td className="mono">{formatTime(ev.timestampUnix)}</td>
                  <td className="mono text-muted">{timeAgo(ev.timestampUnix)}</td>
                  <td><StatusBadge status={ev.severity === "error" ? "failed" : ev.severity === "warning" ? "pending" : "info"} label={ev.severity} /></td>
                  <td className="mono" style={{ fontSize: 12 }}>{ev.type}</td>
                  <td>{ev.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
