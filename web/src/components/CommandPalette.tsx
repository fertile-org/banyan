import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import {
  Search,
  LayoutDashboard,
  Server,
  Rocket,
  Container,
  Cpu,
  Activity,
  ScrollText,
  KeyRound,
  Settings,
  RefreshCw,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { AgentDetail, ContainerInfo, DeploymentInfo } from "@/api/types";
import { listAgents, listContainers, listDeployments } from "@/api/client";

interface PaletteItem {
  id: string;
  name: string;
  desc: string;
  section: string;
  icon: LucideIcon;
  action: () => void;
}

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const setOpen = onOpenChange;
  const [filter, setFilter] = useState("");
  const [cursor, setCursor] = useState(0);
  const navigate = useNavigate();
  const location = useLocation();
  const inputRef = useRef<HTMLInputElement>(null);

  // Lazy data — fetched once when palette opens, not continuously polled
  const [agents, setAgents] = useState<AgentDetail[]>([]);
  const [deployments, setDeployments] = useState<DeploymentInfo[]>([]);
  const [containers, setContainers] = useState<ContainerInfo[]>([]);

  // Fetch data when palette opens
  useEffect(() => {
    if (!open) return;
    void listAgents().then((r) => setAgents(r.agents ?? []));
    void listDeployments().then((r) => setDeployments(r.deployments ?? []));
    void listContainers().then((r) => setContainers(r.containers ?? []));
  }, [open]);

  const items = useMemo(() => {
    const list: PaletteItem[] = [];

    const nav: { to: string; icon: LucideIcon; label: string; desc: string }[] = [
      { to: "/", icon: LayoutDashboard, label: "Overview", desc: "Cluster overview" },
      { to: "/agents", icon: Server, label: "Agents", desc: "List all agents" },
      { to: "/deployments", icon: Rocket, label: "Deployments", desc: "List all deployments" },
      { to: "/containers", icon: Container, label: "Containers", desc: "List all containers" },
      { to: "/engine", icon: Cpu, label: "Engine", desc: "Engine detail & metrics" },
      { to: "/events", icon: Activity, label: "Events", desc: "Recent event log" },
      { to: "/logs", icon: ScrollText, label: "Logs", desc: "Container log viewer" },
      { to: "/secrets", icon: KeyRound, label: "Secrets", desc: "Secret management" },
      { to: "/settings", icon: Settings, label: "Settings", desc: "Configuration" },
    ];

    for (const item of nav) {
      list.push({ id: `nav:${item.to}`, name: item.label, desc: item.desc, section: "Navigate", icon: item.icon, action: () => navigate(item.to) });
    }

    list.push({ id: "cmd:refresh", name: "Refresh", desc: "Refresh page data", section: "Commands", icon: RefreshCw, action: () => window.location.reload() });

    for (const agent of agents) {
      list.push({ id: `agent:${agent.name}`, name: agent.name, desc: `Agent — ${agent.status}`, section: "Agents", icon: Server, action: () => navigate(`/agents/${encodeURIComponent(agent.name)}`) });
    }

    const seen = new Set<string>();
    for (const dep of deployments) {
      if (seen.has(dep.name)) continue;
      seen.add(dep.name);
      list.push({ id: `deploy:${dep.id}`, name: dep.name, desc: `Deployment — ${dep.status} (${dep.healthy ?? 0}/${dep.total ?? 0})`, section: "Deployments", icon: Rocket, action: () => navigate(`/deployments/${encodeURIComponent(dep.id)}`) });
    }

    for (const c of containers) {
      list.push({ id: `container:${c.containerName}`, name: c.containerName, desc: `${c.serviceName} on ${c.agentId} — ${c.containerStatus}`, section: "Containers", icon: Container, action: () => navigate(`/containers/${encodeURIComponent(c.containerName)}`) });
    }

    return list;
  }, [agents, deployments, containers, navigate]);

  const filtered = useMemo(() => {
    if (!filter) return items.filter((i) => i.section === "Navigate" || i.section === "Commands");
    const lower = filter.toLowerCase();
    return items.filter((i) => i.name.toLowerCase().includes(lower) || i.desc.toLowerCase().includes(lower));
  }, [items, filter]);

  useEffect(() => { setCursor(0); }, [filter]);
  useEffect(() => { setOpen(false); setFilter(""); }, [location.pathname, setOpen]);
  useEffect(() => { if (open) requestAnimationFrame(() => inputRef.current?.focus()); }, [open]);

  const handleGlobalKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") { e.preventDefault(); setOpen(!open); setFilter(""); setCursor(0); }
    if (e.key === "Escape" && open) { e.preventDefault(); setOpen(false); }
  }, [open, setOpen]);

  useEffect(() => {
    window.addEventListener("keydown", handleGlobalKeyDown);
    return () => window.removeEventListener("keydown", handleGlobalKeyDown);
  }, [handleGlobalKeyDown]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setCursor((c) => Math.min(c + 1, filtered.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setCursor((c) => Math.max(c - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); const item = filtered[cursor]; if (item) { item.action(); setOpen(false); setFilter(""); } }
  };

  if (!open) return null;

  const sections: { name: string; items: PaletteItem[] }[] = [];
  let lastSection = "";
  for (const item of filtered) {
    if (item.section !== lastSection) { sections.push({ name: item.section, items: [] }); lastSection = item.section; }
    sections[sections.length - 1]!.items.push(item);
  }

  let globalIndex = 0;

  return (
    <div className="palette-overlay" onClick={() => setOpen(false)}>
      <div className="palette-container" onClick={(e) => e.stopPropagation()}>
        <div className="palette-input-wrap">
          <Search size={18} />
          <input ref={inputRef} className="palette-input" placeholder="Search pages, agents, deployments, containers..." value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={handleKeyDown} />
        </div>
        <div className="palette-results">
          {filtered.length === 0 && <div style={{ padding: "24px 16px", textAlign: "center", color: "var(--text-muted)" }}>No matching results</div>}
          {sections.map((section) => (
            <div key={section.name}>
              <div className="palette-section">{section.name}</div>
              {section.items.map((item) => {
                const idx = globalIndex++;
                return (
                  <div key={item.id} className={`palette-item${idx === cursor ? " selected" : ""}`} onClick={() => { item.action(); setOpen(false); setFilter(""); }} onMouseEnter={() => setCursor(idx)}>
                    <item.icon />
                    <span className="palette-item-name">{item.name}</span>
                    <span className="palette-item-desc">{item.desc}</span>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
        <div className="palette-footer">
          <span><kbd>↑↓</kbd> Navigate</span>
          <span><kbd>↵</kbd> Select</span>
          <span><kbd>Esc</kbd> Close</span>
          <span style={{ marginLeft: "auto" }}><kbd>⌘K</kbd> Toggle</span>
        </div>
      </div>
    </div>
  );
}
