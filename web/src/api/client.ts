// Connect protocol client — sends JSON to connect-go endpoints.
// All RPC methods are POST to /banyan.v1.EngineService/<Method>.

import type {
  ClusterOverview,
  AgentDetail,
  DeploymentInfo,
  ContainerInfo,
  ClusterEvent,
  SecretInfo,
  StopTaskRequest,
  StopTaskResponse,
  ScaleRequest,
  ScaleResponse,
  DownRequest,
  DownResponse,
} from "./types";

const BASE_URL = "/banyan.v1.EngineService";

class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Convert snake_case keys to camelCase recursively (safety net for protojson) */
function snakeToCamel(s: string): string {
  return s.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase());
}

function normalizeKeys(obj: unknown): unknown {
  if (Array.isArray(obj)) return obj.map(normalizeKeys);
  if (obj !== null && typeof obj === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj as Record<string, unknown>)) {
      result[snakeToCamel(key)] = normalizeKeys(value);
    }
    return result;
  }
  return obj;
}

async function rpc<TReq, TResp>(method: string, request: TReq): Promise<TResp> {
  const res = await fetch(`${BASE_URL}/${method}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });

  if (!res.ok) {
    const body = await res.text();
    let code = "unknown";
    let message = body;
    try {
      const parsed = JSON.parse(body) as { code?: string; message?: string };
      code = parsed.code ?? code;
      message = parsed.message ?? message;
    } catch {
      // body is not JSON
    }
    throw new ApiError(code, message);
  }

  const raw = await res.json();
  return normalizeKeys(raw) as TResp;
}

// --- Per-page APIs ---

export function getClusterOverview(): Promise<ClusterOverview> {
  return rpc<object, ClusterOverview>("GetClusterOverview", {});
}

export function listAgents(): Promise<{ agents?: AgentDetail[] }> {
  return rpc("ListAgents", {});
}

export function listDeployments(): Promise<{ deployments?: DeploymentInfo[] }> {
  return rpc("ListDeployments", {});
}

export function getDeploymentDetail(id: string): Promise<{ deployment?: DeploymentInfo }> {
  return rpc("GetDeploymentDetail", { id });
}

export function listContainers(): Promise<{ containers?: ContainerInfo[] }> {
  return rpc("ListContainers", {});
}

export function listEvents(limit?: number): Promise<{ events?: ClusterEvent[] }> {
  return rpc("ListEvents", { limit: limit ?? 0 });
}

export function getRecentLogs(containerName: string, tail: number = 500, agentId?: string): Promise<{ lines?: string[]; containerName?: string }> {
  return rpc("GetRecentLogs", { containerName, tail, agentId: agentId || "" });
}

// --- Secret APIs ---

export function listSecrets(): Promise<{ secrets?: SecretInfo[] }> {
  return rpc("ListSecrets", {});
}

export function createSecret(name: string, value: string): Promise<object> {
  return rpc("CreateSecret", { name, value });
}

export function getSecret(name: string, reveal: boolean): Promise<SecretInfo> {
  return rpc("GetSecret", { name, reveal });
}

export function deleteSecret(name: string): Promise<object> {
  return rpc("DeleteSecret", { name });
}

// --- Action APIs ---

export function stopTask(req: StopTaskRequest): Promise<StopTaskResponse> {
  return rpc("StopTask", req);
}

export function scale(req: ScaleRequest): Promise<ScaleResponse> {
  return rpc("Scale", req);
}

export function teardown(req: DownRequest): Promise<DownResponse> {
  return rpc("Down", req);
}

export function healthCheck(): Promise<{ status: string }> {
  return rpc("Health", {});
}

// --- Log streaming ---

export async function* streamLogs(
  containerName: string,
  tail: number = 100,
  agentId?: string,
): AsyncGenerator<string> {
  const res = await fetch(`${BASE_URL}/GetLogs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ containerName, follow: true, tail, agentId: agentId || "" }),
  });

  if (!res.ok || !res.body) {
    throw new ApiError("unavailable", "Failed to connect to log stream");
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (!line.trim()) continue;
      try {
        const msg = JSON.parse(line) as { result?: { data?: string } };
        if (msg.result?.data) {
          yield atob(msg.result.data);
        }
      } catch {
        yield line;
      }
    }
  }
}

export { ApiError };
