# Engine Modes — Technical Flow Diagrams

## Overview

Banyan supports two deployment modes:

- **Single-engine** — one engine process manages everything (default)
- **Multi-engine HA** — 2+ engines share state via external etcd, with leader-based scheduling

Both modes use the same agent and CLI binaries. The difference is in how engines coordinate.

---

## 1. Single-Engine Mode

### Startup Flow

```mermaid
sequenceDiagram
    participant E as Engine
    participant Etcd as Managed etcd
    participant Reg as Managed Registry
    participant GRPC as gRPC Server
    participant EL as Orchestration Loop

    E->>Etcd: Start managed etcd subprocess
    E->>Etcd: Wait for health check (127.0.0.1:2379)
    E->>E: Init VPC (in-memory SubnetAllocator + PeerTracker)
    E->>Reg: Start managed registry subprocess
    E->>Reg: Wait for health check (/v2/)
    E->>Etcd: Save registry URL to etcd
    E->>GRPC: Start gRPC server (WireGuard tunnel IP)
    E->>Etcd: Register engine record (KeepAlive, 15s TTL)
    E->>EL: Start orchestration loop (every 3s)
    EL-->>EL: processDeployments() — always runs
    EL-->>EL: updateMetrics()
```

### Request Flow (Deploy)

```mermaid
sequenceDiagram
    participant CLI as CLI
    participant E as Engine gRPC
    participant Etcd as etcd
    participant A1 as Agent 1
    participant A2 as Agent 2

    CLI->>E: Deploy(manifest)
    E->>Etcd: Save DeploymentRecord (status: PENDING)
    E-->>CLI: DeploymentID

    Note over E: Orchestration loop picks up PENDING deployment (3s tick)

    E->>Etcd: List available agents
    E->>E: Schedule tasks (resource-aware)
    E->>Etcd: Save TaskRecords (status: PENDING)
    E->>Etcd: Update deployment (status: DEPLOYING)

    A1->>E: PollTasks()
    E-->>A1: [task-1: create web-0]
    A1->>A1: Pull image, start container
    A1->>E: ReportTaskResult(completed)

    A2->>E: PollTasks()
    E-->>A2: [task-2: create web-1]
    A2->>A2: Pull image, start container
    A2->>E: ReportTaskResult(completed)

    Note over E: Orchestration loop detects all tasks completed
    E->>Etcd: Update deployment (status: RUNNING)
```

### Agent Connection Flow

```mermaid
sequenceDiagram
    participant A as Agent
    participant WG as WireGuard Tunnel
    participant E as Engine gRPC

    A->>WG: Setup control tunnel (wg-ctl-agt)
    A->>E: Connect via tunnel IP (10.200.0.1:50051)
    A->>E: Health() — wait for ready
    A->>E: Register(name, hostIP, wgPubKey)
    E-->>A: registryURL, allocatedSubnet, activeContainers
    A->>A: Init VPC overlay (VXLAN + bridge)

    rect rgb(240, 248, 255)
        Note over A,E: Heartbeat cycle (every 15s)
        A->>E: Heartbeat(metrics, tags)
        E-->>A: vpcPeers, serviceBackends
        A->>A: Reconcile VPC peers
        A->>A: Reconcile DNS records
        A->>A: Reconcile proxy rules
    end

    rect rgb(245, 245, 245)
        Note over A,E: Task polling cycle (every 3s)
        A->>E: PollTasks()
        E-->>A: pending tasks
        A->>A: Execute tasks (pull, start, stop)
        A->>E: ReportTaskResult()
    end
```

---

## 2. Multi-Engine HA Mode

### Architecture

All engines are **identical processes**. Every engine handles RPCs AND runs the scheduling loop. Per-deployment distributed locks in etcd prevent duplicate work. There is no leader — all engines are active.

If an engine dies, the others continue immediately. No election, no failover delay.

```mermaid
graph LR
    subgraph Clients
        CLI[banyan-cli]
        A1[Agent 1]
        A2[Agent 2]
    end

    subgraph Engines — all identical, all active
        E1["Engine 1<br/>gRPC + Scheduling"]
        E2["Engine 2<br/>gRPC + Scheduling"]
    end

    subgraph Shared State
        Etcd[(etcd cluster)]
        Reg[OCI Registry]
    end

    CLI -->|"connects to any"| E1
    CLI -.->|"failover"| E2
    A1 -->|"connects to any"| E1
    A1 -.->|"failover"| E2
    A2 -->|"connects to any"| E2
    A2 -.->|"failover"| E1

    E1 <-->|"per-deployment locks"| Etcd
    E2 <-->|"per-deployment locks"| Etcd
    E1 --> Reg
    E2 --> Reg
```

### How requests reach engines

Clients connect to any engine. The engine that receives the Deploy RPC writes to etcd AND schedules immediately. No leader, no forwarding, no delay.

```mermaid
sequenceDiagram
    participant CLI as banyan-cli
    participant E2 as Engine 2
    participant Etcd as etcd
    participant E1 as Engine 1

    Note over CLI: CLI config has engine_endpoints: [E2, E1]
    Note over CLI: CLI tries E2 first, health check passes

    CLI->>E2: Deploy(manifest)
    E2->>Etcd: Write DeploymentRecord (status: PENDING)
    E2-->>CLI: OK, deployment_id=abc123

    Note over E2: E2 triggers immediate scheduling
    E2->>Etcd: Lock("locks/deploy/abc123")
    E2->>E2: Schedule tasks for abc123
    E2->>Etcd: Write tasks, update status to DEPLOYING
    E2->>Etcd: Unlock

    Note over E1: E1 loop also sees abc123 (3s later)
    E1->>Etcd: Lock("locks/deploy/abc123")
    E1->>Etcd: Re-read abc123 status
    Note over E1: Status is DEPLOYING, not PENDING — skip
    E1->>Etcd: Unlock

    Note over CLI: Later, CLI checks status (could hit either engine)
    CLI->>E1: GetStatus()
    E1->>Etcd: Read deployment abc123
    E1-->>CLI: status: DEPLOYING
```

### Startup Flow (per engine)

Every engine starts the same way. No leader election — all engines are active.

```mermaid
sequenceDiagram
    participant E as Engine
    participant Etcd as External etcd
    participant GRPC as gRPC Server
    participant EL as Orchestration Loop

    E->>Etcd: Connect to external etcd
    E->>E: Init VPC (etcd-backed SubnetAllocator + PeerTracker)
    E->>E: Set registry URL from config (external)
    E->>Etcd: Save registry URL
    E->>GRPC: Start gRPC server
    E->>Etcd: Register engine (KeepAlive, 15s TTL)
    E->>EL: Start orchestration loop (every 3s)
    EL-->>EL: processDeployments() with per-deployment locks
    EL-->>EL: updateMetrics()

    Note over GRPC: Deploy/Register RPCs trigger immediate scheduling via channel
```

### Engine Failover (Active-Active)

No leader election means no failover delay. When an engine dies, the others continue immediately.

```mermaid
sequenceDiagram
    participant E1 as Engine 1
    participant E2 as Engine 2
    participant Etcd as etcd

    Note over E1,E2: Both engines active, both scheduling

    E1->>Etcd: Lock deploy/A, schedule A
    E2->>Etcd: Lock deploy/B, schedule B
    Note over E1,E2: Work distributed naturally via locks

    Note over E1: Engine 1 crashes
    Note over E1: Engine 1 locks auto-expire (30s TTL)
    Note over E2: Engine 2 continues uninterrupted
    E2->>Etcd: Lock deploy/A (expired), schedule A if needed
    E2->>Etcd: Lock deploy/B, schedule B

    Note over E1: Engine 1 restarts
    E1->>Etcd: Register engine (KeepAlive)
    E1->>Etcd: Start orchestration loop
    Note over E1,E2: Both engines active again
```

### Scheduling Triggers

Scheduling happens via three triggers. Per-deployment locks ensure no duplicate work.

```mermaid
sequenceDiagram
    participant CLI as CLI
    participant E2 as Engine 2
    participant Etcd as etcd
    participant E1 as Engine 1

    Note over E2: Trigger 1: Deploy RPC (instant)
    CLI->>E2: Deploy(manifest)
    E2->>Etcd: Write deployment (PENDING)
    E2->>E2: triggerSchedule via channel
    E2->>Etcd: Lock("locks/deploy/abc")
    E2->>E2: Schedule tasks
    E2->>Etcd: Write tasks, update to DEPLOYING
    E2->>Etcd: Unlock
    E2-->>CLI: deployment_id=abc

    Note over E1: Trigger 2: Loop (safety net, every 3s)
    E1->>Etcd: List PENDING deployments
    E1->>Etcd: Lock("locks/deploy/abc")
    E1->>Etcd: Re-read: status is DEPLOYING
    Note over E1: Already handled, skip
    E1->>Etcd: Unlock

    Note over E1: Trigger 3: Agent registration (new capacity)
    E1->>E1: New agent registered, triggerSchedule
    E1->>Etcd: List PENDING deployments
    Note over E1: Schedule anything waiting for resources
```

### Agent Multi-Endpoint Failover

```mermaid
sequenceDiagram
    participant A as Agent
    participant E1 as Engine 1
    participant E2 as Engine 2

    Note over A: Config: engine_endpoints: [E1:50051, E2:50051]

    A->>E1: Connect (endpoint 0)
    A->>E1: Register()
    E1-->>A: OK

    rect rgb(240, 248, 255)
        Note over A,E1: Normal operation
        A->>E1: Heartbeat()
        A->>E1: PollTasks()
    end

    Note over E1: Engine 1 goes down

    A->>E1: Heartbeat() fails
    A->>E1: Heartbeat() fails (3 consecutive)

    Note over A: Trigger reconnect - try all endpoints

    A->>E1: Health() timeout
    Note over A: Failover to next endpoint
    A->>E2: Health() OK
    A->>E2: Register()
    E2-->>A: OK (subnet, peers, active containers)

    rect rgb(245, 245, 245)
        Note over A,E2: Resumes on Engine 2
        A->>E2: Heartbeat()
        A->>E2: PollTasks()
    end
```

### VPC State Coordination (etcd-backed)

```mermaid
sequenceDiagram
    participant A1 as Agent 1
    participant E1 as Engine 1
    participant Etcd as etcd
    participant E2 as Engine 2
    participant A2 as Agent 2

    A1->>E1: Register(worker-1)
    E1->>Etcd: Lock("locks/subnet-allocator")
    E1->>Etcd: List /banyan/vpc/subnets/
    E1->>E1: Find available /24
    E1->>Etcd: Save /banyan/vpc/subnets/worker-1 = "10.0.0.0/24"
    E1->>Etcd: Unlock
    E1->>Etcd: Save /banyan/vpc/peers/worker-1 = {subnet, hostIP}
    E1-->>A1: allocatedSubnet: 10.0.0.0/24

    A2->>E2: Register(worker-2)
    Note over E2: E2 handles this RPC (any engine can)
    E2->>Etcd: Lock("locks/subnet-allocator")
    E2->>Etcd: List /banyan/vpc/subnets/
    Note over E2: Sees 10.0.0.0/24 already taken
    E2->>Etcd: Save /banyan/vpc/subnets/worker-2 = "10.0.1.0/24"
    E2->>Etcd: Unlock
    E2->>Etcd: Save /banyan/vpc/peers/worker-2 = {subnet, hostIP}
    E2-->>A2: allocatedSubnet: 10.0.1.0/24

    Note over A1,A2: Both agents get peer lists via heartbeat from any engine
    A1->>E1: Heartbeat()
    E1->>Etcd: List /banyan/vpc/peers/ exclude worker-1
    E1-->>A1: peers: [worker-2, 10.0.1.0/24, hostIP]

    A2->>E2: Heartbeat()
    E2->>Etcd: List /banyan/vpc/peers/ exclude worker-2
    E2-->>A2: peers: [worker-1, 10.0.0.0/24, hostIP]
```

---

## 3. Mode Comparison

| Aspect | Single-Engine | Multi-Engine HA |
|--------|--------------|-----------------|
| **etcd** | Managed subprocess | External cluster (user-provided) |
| **Registry** | Managed subprocess | External registry (user-provided) |
| **Scheduling** | Direct (no locking) | Active-active + per-deployment distributed locks |
| **VPC state** | In-memory maps | etcd-backed (CAS + locks) |
| **Agent connection** | Single endpoint | Multiple endpoints with failover |
| **Failover time** | N/A (single point) | **Instant** (no leader election, lock TTL only) |
| **Config complexity** | Zero (all defaults) | Requires external etcd + registry URLs |

### etcd Key Schema

```
/banyan/
├── deployments/{id}          # DeploymentRecord (shared)
├── nodes/{name}              # NodeRecord (shared)
├── tasks/{agent}/{id}        # TaskRecord (shared)
├── registry                  # Registry URL (shared)
├── engines/{id}              # EngineRecord (15s TTL lease)
└── vpc/
    ├── subnets/{agent}       # Allocated /24 subnet (multi-engine only)
    └── peers/{agent}         # Peer overlay info (multi-engine only)
```
