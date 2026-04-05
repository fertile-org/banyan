import { Activity, RefreshCw } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { FilterSelect } from "@/components/FilterSelect";
import { timeAgo, formatTime } from "@/lib/format";
import { useEvents } from "@/hooks/use-api";
import { useState, useMemo } from "react";

const PAGE_SIZE = 50;

export function Events() {
  const [limit, setLimit] = useState(PAGE_SIZE);
  const { data: events, refresh } = useEvents(limit);
  const [typeFilter, setTypeFilter] = useState("");
  const [severityFilter, setSeverityFilter] = useState("");

  const filterOptions = useMemo(() => {
    const all = events ?? [];
    return {
      types: [...new Set(all.map((e) => e.type))].sort(),
      severities: [...new Set(all.map((e) => e.severity))].sort(),
    };
  }, [events]);

  const filtered = (events ?? []).filter((ev) => {
    if (typeFilter && ev.type !== typeFilter) return false;
    if (severityFilter && ev.severity !== severityFilter) return false;
    return true;
  });

  const hasFilters = !!(typeFilter || severityFilter);
  const canLoadMore = events && events.length >= limit;
  const totalCount = events?.length ?? 0;

  const clearFilters = () => {
    setTypeFilter("");
    setSeverityFilter("");
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title"><Activity size={22} /> Events</h1>
        <div className="header-actions">
          <button className="icon-btn" onClick={refresh} title="Refresh"><RefreshCw size={16} /></button>
        </div>
      </div>

      <div className="filter-bar">
        <FilterSelect value={typeFilter} onChange={setTypeFilter} options={filterOptions.types} placeholder="All types" width={180} />
        <FilterSelect value={severityFilter} onChange={setSeverityFilter} options={filterOptions.severities} placeholder="All severities" width={140} />
        {hasFilters && (
          <>
            <span className="filter-info">Showing {filtered.length} of {totalCount}</span>
            <button className="btn btn-secondary" onClick={clearFilters} style={{ fontSize: 12, padding: "4px 10px" }}>Clear</button>
          </>
        )}
      </div>

      <div className="panel">
        <div className="panel-header">
          <div className="panel-title"><Activity size={16} /> Recent Events <span className="panel-count">{filtered.length}</span></div>
        </div>
        {!filtered.length ? (
          <div className="empty-state"><Activity /><h3>No events</h3><p>{hasFilters ? "No events match your filters." : "Events will appear here as the cluster operates."}</p></div>
        ) : (
          <>
            <table>
              <thead><tr><th style={{ width: 80 }}>Time</th><th style={{ width: 80 }}>Ago</th><th style={{ width: 80 }}>Severity</th><th style={{ width: 200 }}>Type</th><th>Message</th></tr></thead>
              <tbody>
                {filtered.map((ev, i) => (
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
            {canLoadMore && (
              <div style={{ padding: 16, textAlign: "center" }}>
                <button className="btn btn-secondary" onClick={() => setLimit((l) => l + PAGE_SIZE)}>
                  Load More
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </>
  );
}
