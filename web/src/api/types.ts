// TypeScript types matching the protobuf messages from engine.proto.
// Field names use camelCase (proto3 JSON / protojson convention).
// Many fields are optional because protojson omits zero/default values
// (though our custom codec emits all fields now).

export interface SystemMetrics {
  cpuUsageRatio?: number;
  memoryUsedBytes?: string;
  memoryTotalBytes?: string;
  diskUsedBytes?: string;
  diskTotalBytes?: string;
  cpuCores?: number;
}

export interface EngineStatus {
  status: string;
  startedAtUnix: string;
  systemMetrics?: SystemMetrics;
}

export interface AgentDetail {
  name: string;
  status: string;
  apiAddress?: string;
  lastSeenUnix: string;
  createdAtUnix: string;
  tags?: string[];
  systemMetrics?: SystemMetrics;
  vpcSubnet?: string;
  containerCount?: number;
}

export interface ServiceInfo {
  image: string;
  replicas: number;
  ports?: string[];
  command?: string[];
  dependsOn?: Record<string, { condition: string }>;
}

export interface TaskInfo {
  id: string;
  deploymentId: string;
  serviceName: string;
  replicaIndex?: number;
  agentId: string;
  type: string;
  status: string;
  image: string;
  containerName: string;
  ports?: string[];
  command?: string[];
  containerStatus?: string;
  healthStatus?: string;
  containerCheckedAtUnix?: string;
  createdAtUnix: string;
  updatedAtUnix?: string;
  error?: string;
  cpuPercent?: number;
  memoryUsedBytes?: string;
  memoryLimitBytes?: string;
}

export interface DeploymentInfo {
  id: string;
  name: string;
  status: string;
  services?: Record<string, ServiceInfo>;
  createdAtUnix: string;
  updatedAtUnix?: string;
  error?: string;
  tasks?: TaskInfo[];
  healthy?: number;
  total?: number;
  tags?: string[];
}

export interface ClusterSummary {
  totalAgents?: number;
  connectedAgents?: number;
  totalDeployments?: number;
  runningDeployments?: number;
  totalContainers?: number;
  healthyContainers?: number;
  tasksByStatus?: Record<string, number>;
}

export interface ClusterEvent {
  timestampUnix: string;
  type: string;
  message: string;
  severity: string;
}

// --- Per-page response types ---

export interface ClusterOverview {
  engine?: EngineStatus;
  summary?: ClusterSummary;
  recentEvents?: ClusterEvent[];
}

export interface ContainerInfo {
  taskId: string;
  containerName: string;
  serviceName: string;
  replicaIndex?: number;
  agentId: string;
  deploymentName: string;
  deploymentId: string;
  image: string;
  containerStatus: string;
  healthStatus?: string;
  ports?: string[];
  cpuPercent?: number;
  memoryUsedBytes?: string;
  memoryLimitBytes?: string;
  createdAtUnix: string;
  updatedAtUnix?: string;
  error?: string;
}

// --- Legacy monolithic response (kept for CLI TUI compatibility) ---

export interface DashboardData {
  engine?: EngineStatus;
  agents?: AgentDetail[];
  deployments?: DeploymentInfo[];
  summary?: ClusterSummary;
  recentEvents?: ClusterEvent[];
}

// --- Action request/response types ---

export interface StopTaskRequest {
  taskId: string;
  agentId: string;
}

export interface StopTaskResponse {
  stopTaskId: string;
  status: string;
}

export interface ScaleRequest {
  name: string;
  replicas: Record<string, number>;
  tags: string[];
}

export interface ScaleResponse {
  deploymentId: string;
  previous: Record<string, number>;
  current: Record<string, number>;
}

export interface DownRequest {
  name: string;
  services: string[];
  tags: string[];
}

export interface DownResponse {
  taskCount: number;
}
