import type { LucideIcon } from "lucide-react";

interface StatCardProps {
  label: string;
  icon: LucideIcon;
  value: number;
  total?: number;
  valueColor?: string;
  subIcon?: LucideIcon;
  subText: string;
  subColor?: string;
}

export function StatCard({
  label,
  icon: Icon,
  value,
  total,
  valueColor = "var(--text)",
  subIcon: SubIcon,
  subText,
  subColor = "var(--text-secondary)",
}: StatCardProps) {
  return (
    <div className="stat-card">
      <div className="stat-card-header">
        <div className="stat-label">{label}</div>
        <Icon size={16} className="stat-icon" />
      </div>
      <div className="stat-value" style={{ color: valueColor }}>
        {value}
        {total !== undefined && (
          <span className="stat-total"> / {total}</span>
        )}
      </div>
      <div className="stat-sub">
        {SubIcon && <SubIcon size={11} style={{ color: subColor }} />}
        <span style={{ color: subColor }}>{subText}</span>
      </div>
    </div>
  );
}
