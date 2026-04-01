import { useRef } from "react";
import type { ContainerInfo } from "@/api/types";

const MAX_SAMPLES = 20;

/**
 * Accumulates CPU history across polling intervals for sparkline display.
 *
 * Uses a ref (not state) to avoid triggering extra re-renders.
 * The parent component already re-renders when `containers` changes (from usePolling),
 * so this hook just needs to update the ref during that render — no additional
 * state update needed.
 */
export function useCPUHistory(containers: ContainerInfo[] | null): Record<string, number[]> {
  const historyRef = useRef<Record<string, number[]>>({});
  const prevContainersRef = useRef<ContainerInfo[] | null>(null);

  // Only update history when containers reference actually changes
  if (containers && containers !== prevContainersRef.current) {
    prevContainersRef.current = containers;

    const current = new Set<string>();
    for (const c of containers) {
      if (!c.containerName) continue;
      current.add(c.containerName);
      const existing = historyRef.current[c.containerName];
      const arr = existing ? [...existing, c.cpuPercent ?? 0] : [c.cpuPercent ?? 0];
      if (arr.length > MAX_SAMPLES) {
        arr.splice(0, arr.length - MAX_SAMPLES);
      }
      historyRef.current[c.containerName] = arr;
    }

    // Prune removed containers
    for (const name of Object.keys(historyRef.current)) {
      if (!current.has(name)) {
        delete historyRef.current[name];
      }
    }
  }

  return historyRef.current;
}
