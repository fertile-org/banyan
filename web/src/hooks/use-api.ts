// Per-page data fetching hooks.
// Each hook polls its own RPC endpoint independently.
// Only the active page polls — no global background polling.

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getClusterOverview,
  listAgents,
  listDeployments,
  getDeploymentDetail,
  listContainers,
  listEvents,
  healthCheck,
} from "@/api/client";
import type {
  ClusterOverview,
  AgentDetail,
  DeploymentInfo,
  ContainerInfo,
  ClusterEvent,
} from "@/api/types";

const POLL_INTERVAL = 10_000;

interface UseApiResult<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

/** Generic polling hook — fetches on mount, then at interval. */
function usePolling<T>(fetcher: () => Promise<T>, interval = POLL_INTERVAL): UseApiResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  const fetchData = useCallback(async () => {
    try {
      const result = await fetcherRef.current();
      setData(result);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch");
    } finally {
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(() => {
    setLoading(true);
    void fetchData();
  }, [fetchData]);

  useEffect(() => {
    void fetchData();
    const id = setInterval(() => void fetchData(), interval);
    return () => clearInterval(id);
  }, [fetchData, interval]);

  return { data, loading, error, refresh };
}

// --- Page-specific hooks ---

export function useOverview(): UseApiResult<ClusterOverview> {
  return usePolling(getClusterOverview);
}

export function useAgents(): UseApiResult<AgentDetail[]> {
  const fetcher = useCallback(async () => {
    const resp = await listAgents();
    return resp.agents ?? [];
  }, []);
  return usePolling(fetcher);
}

export function useDeployments(): UseApiResult<DeploymentInfo[]> {
  const fetcher = useCallback(async () => {
    const resp = await listDeployments();
    return resp.deployments ?? [];
  }, []);
  return usePolling(fetcher);
}

export function useDeploymentDetail(id: string): UseApiResult<DeploymentInfo | null> {
  const fetcher = useCallback(async () => {
    const resp = await getDeploymentDetail(id);
    return resp.deployment ?? null;
  }, [id]);
  return usePolling(fetcher);
}

export function useContainers(): UseApiResult<ContainerInfo[]> {
  const fetcher = useCallback(async () => {
    const resp = await listContainers();
    return resp.containers ?? [];
  }, []);
  return usePolling(fetcher);
}

export function useEvents(limit?: number): UseApiResult<ClusterEvent[]> {
  const fetcher = useCallback(async () => {
    const resp = await listEvents(limit);
    return resp.events ?? [];
  }, [limit]);
  return usePolling(fetcher);
}

export function useEngineHealth(): { connected: boolean } {
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    const check = async () => {
      try {
        await healthCheck();
        setConnected(true);
      } catch {
        setConnected(false);
      }
    };
    void check();
    const id = setInterval(() => void check(), 15_000);
    return () => clearInterval(id);
  }, []);

  return { connected };
}
