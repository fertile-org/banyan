// Formatting utilities for the dashboard

/** Format bytes to human-readable string */
export function formatBytes(bytes: string | number): string {
  const n = typeof bytes === "string" ? parseInt(bytes, 10) : bytes;
  if (!n || isNaN(n)) return "-";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(0)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

/** Format a unix timestamp (seconds) to relative time */
export function timeAgo(unixSeconds: string | number): string {
  const ts = typeof unixSeconds === "string" ? parseInt(unixSeconds, 10) : unixSeconds;
  if (!ts || isNaN(ts)) return "-";
  const diff = Math.floor(Date.now() / 1000 - ts);
  if (diff < 0) return "just now";
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

/** Format a unix timestamp (seconds) to a time string */
export function formatTime(unixSeconds: string | number): string {
  const ts = typeof unixSeconds === "string" ? parseInt(unixSeconds, 10) : unixSeconds;
  if (!ts || isNaN(ts)) return "-";
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString("en-US", { hour12: false });
}

/** Format CPU percentage */
export function formatCPU(percent: number): string {
  if (!percent) return "-";
  return `${percent.toFixed(1)}%`;
}

/** Format percentage from ratio (0-1) */
export function formatRatio(ratio: number): string {
  if (!ratio && ratio !== 0) return "-";
  return `${(ratio * 100).toFixed(1)}%`;
}
