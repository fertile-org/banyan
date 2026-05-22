import { NavLink } from "react-router-dom";
import {
  LayoutDashboard,
  Server,
  Rocket,
  Container,
  Cpu,
  Activity,
  ScrollText,
  KeyRound,
  Settings,
  CircleDot,
  Search,
  LogOut,
} from "lucide-react";
import { useEngineHealth } from "@/hooks/use-api";

interface SidebarProps {
  onOpenPalette: () => void;
  username: string | null;
  role: string | null;
  onLogout: () => void;
}

const clusterNav = [
  { to: "/", icon: LayoutDashboard, label: "Overview" },
  { to: "/agents", icon: Server, label: "Agents" },
  { to: "/deployments", icon: Rocket, label: "Deployments" },
  { to: "/containers", icon: Container, label: "Containers" },
  { to: "/engine", icon: Cpu, label: "Engine" },
];

const opsNav = [
  { to: "/events", icon: Activity, label: "Events" },
  { to: "/logs", icon: ScrollText, label: "Logs" },
  { to: "/secrets", icon: KeyRound, label: "Secrets" },
];

const settingsNav = [
  { to: "/settings", icon: Settings, label: "Configuration" },
];

export function Sidebar({ onOpenPalette, username, role, onLogout }: SidebarProps) {
  const { connected } = useEngineHealth();

  return (
    <nav className="sidebar">
      <div className="sidebar-brand">
        <img src="/logo.png" alt="Banyan" width={24} height={24} style={{ borderRadius: 4 }} />
        <span>Banyan</span>
      </div>

      <button
        className="sidebar-search"
        onClick={onOpenPalette}
        title="Command palette (Ctrl+K)"
      >
        <Search size={14} />
        <span>Search...</span>
        <kbd>⌘K</kbd>
      </button>

      <div className="sidebar-section">Cluster</div>
      {clusterNav.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.to === "/"}
          className={({ isActive }) =>
            `sidebar-item${isActive ? " active" : ""}`
          }
        >
          <item.icon />
          {item.label}
        </NavLink>
      ))}

      <div className="sidebar-section">Operations</div>
      {opsNav.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          className={({ isActive }) =>
            `sidebar-item${isActive ? " active" : ""}`
          }
        >
          <item.icon />
          {item.label}
        </NavLink>
      ))}

      <div className="sidebar-section">Settings</div>
      {settingsNav.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          className={({ isActive }) =>
            `sidebar-item${isActive ? " active" : ""}`
          }
        >
          <item.icon />
          {item.label}
        </NavLink>
      ))}

      {username && (
        <div className="sidebar-footer" style={{ borderBottom: "1px solid var(--border)", paddingBottom: 8, marginBottom: 8 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="sidebar-footer-text" style={{ fontWeight: 500 }}>{username}</div>
            <div className="sidebar-footer-text" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--text-muted)" }}>{role}</div>
          </div>
          <button
            onClick={onLogout}
            title="Sign out"
            style={{ background: "none", border: "none", cursor: "pointer", padding: 4, color: "var(--text-muted)" }}
          >
            <LogOut size={14} />
          </button>
        </div>
      )}

      <div className="sidebar-footer">
        <CircleDot
          size={12}
          style={{ color: connected ? "var(--green)" : "var(--red)" }}
        />
        <div>
          <div className="sidebar-footer-text">engine</div>
          <div
            className="sidebar-footer-text"
            style={{ color: connected ? "var(--green)" : "var(--red)" }}
          >
            {connected ? "connected" : "disconnected"}
          </div>
        </div>
      </div>
    </nav>
  );
}
