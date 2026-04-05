import { useState, useCallback } from "react";
import { Routes, Route } from "react-router-dom";
import { Sidebar } from "@/components/Sidebar";
import { CommandPalette } from "@/components/CommandPalette";
import { Overview } from "@/pages/Overview";
import { Agents } from "@/pages/Agents";
import { AgentDetail } from "@/pages/AgentDetail";
import { Deployments } from "@/pages/Deployments";
import { DeploymentDetail } from "@/pages/DeploymentDetail";
import { Containers } from "@/pages/Containers";
import { ContainerDetail } from "@/pages/ContainerDetail";
import { EnginePage } from "@/pages/Engine";
import { Events } from "@/pages/Events";
import { Logs } from "@/pages/Logs";
import { Secrets } from "@/pages/Secrets";
import { Configuration } from "@/pages/Configuration";
import { ThemeToggle } from "@/components/ThemeToggle";

export function App() {
  const [paletteOpen, setPaletteOpen] = useState(false);
  const openPalette = useCallback(() => setPaletteOpen(true), []);

  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      <Sidebar onOpenPalette={openPalette} />
      <main className="main">
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/agents" element={<Agents />} />
          <Route path="/agents/:name" element={<AgentDetail />} />
          <Route path="/deployments" element={<Deployments />} />
          <Route path="/deployments/:id" element={<DeploymentDetail />} />
          <Route path="/containers" element={<Containers />} />
          <Route path="/containers/:name" element={<ContainerDetail />} />
          <Route path="/engine" element={<EnginePage />} />
          <Route path="/events" element={<Events />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/secrets" element={<Secrets />} />
          <Route path="/settings" element={<Configuration />} />
        </Routes>
        <ThemeToggle />
      </main>
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </div>
  );
}
