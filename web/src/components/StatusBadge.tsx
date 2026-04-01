interface StatusBadgeProps {
  status: string;
  label?: string;
}

export function StatusBadge({ status, label }: StatusBadgeProps) {
  const normalized = status.toLowerCase().replace(/[^a-z]/g, "");
  const displayLabel = label ?? status;

  return (
    <span className={`status status-${normalized}`}>
      <span className="status-dot" />
      {displayLabel}
    </span>
  );
}
