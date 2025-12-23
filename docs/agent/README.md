# Agent Component Designs

This folder contains detailed design documents for each Agent component.

## Overview

The Agent is the **data plane** of Banyan. It runs on each worker node and executes tasks received from the Engine, managing containers, networking, and security at the node level.

## Architecture Pattern

All components follow **Clean Architecture** with **Hexagonal Architecture** (Ports and Adapters):

```
┌─────────────────────────────────────────────────────────┐
│                 Driving Adapters                         │
│  (gRPC handlers, Event handlers)                        │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                   Inbound Ports                          │
│  (Service interfaces - what the component offers)       │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                    Use Cases                             │
│  (Application logic - orchestrates domain)              │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                  Domain Layer                            │
│  (Entities, Value Objects, Domain Logic)                │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                  Outbound Ports                          │
│  (Repository interfaces - what the component needs)     │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                  Driven Adapters                         │
│  (Docker SDK, CNI executors, iptables, filesystem)      │
└─────────────────────────────────────────────────────────┘
```

## Components

| Component | Description | Document |
|-----------|-------------|----------|
| **Container Runtime** | Manages container lifecycle via Docker/containerd | [container-runtime.md](./container-runtime.md) |
| **Network Node** | Configures node-level networking (CNI, routes) | [network-node.md](./network-node.md) |
| **Security Executor** | Applies security policies via iptables/nftables | [security-executor.md](./security-executor.md) |
| **Task Executor** | Executes tasks received from Engine | [task-executor.md](./task-executor.md) |
| **Health Monitor** | Monitors container and node health | [health-monitor.md](./health-monitor.md) |

## Directory Structure

```
pkg/agent/
├── agent.go                  # Main Agent implementation
├── types.go                  # Shared types
├── runtime/
│   ├── domain/
│   │   ├── entities.go
│   │   └── value_objects.go
│   ├── ports/
│   │   ├── inbound.go
│   │   └── outbound.go
│   ├── usecases/
│   │   └── container.go
│   └── adapters/
│       ├── grpc_handler.go
│       └── docker_client.go
├── network/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── security/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── executor/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
└── health/
    ├── domain/
    ├── ports/
    ├── usecases/
    └── adapters/
```

## Component Interactions

```
                                    gRPC
                              [ Engine ]
                                   │
┌──────────────────────────────────┼──────────────────────────────────┐
│                                Agent                                 │
│                                  │                                   │
│  ┌───────────────────────────────▼───────────────────────────────┐  │
│  │                      Task Executor                             │  │
│  │  (Receives and dispatches tasks from Engine)                  │  │
│  └───────┬───────────────┬───────────────┬───────────────────────┘  │
│          │               │               │                           │
│          ▼               ▼               ▼                           │
│  ┌───────────────┐ ┌───────────┐ ┌───────────────┐                  │
│  │   Container   │ │  Network  │ │   Security    │                  │
│  │    Runtime    │ │   Node    │ │   Executor    │                  │
│  └───────┬───────┘ └─────┬─────┘ └───────┬───────┘                  │
│          │               │               │                           │
│          └───────────────┼───────────────┘                           │
│                          │                                           │
│  ┌───────────────────────▼───────────────────────────────────────┐  │
│  │                    Health Monitor                              │  │
│  │  (Monitors all components and reports status)                 │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                         │
         ┌───────────────┼───────────────┐
         ▼               ▼               ▼
    [ Docker ]     [ CNI/VPC ]    [ iptables ]
```

## Agent Lifecycle

```
┌─────────────────────────────────────────────────────────────────────┐
│                      Agent Startup Sequence                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. Initialize Configuration                                         │
│     └─► Load agent config, node info, certificates                  │
│                                                                      │
│  2. Initialize Components                                            │
│     ├─► Container Runtime (connect to Docker/containerd)            │
│     ├─► Network Node (configure CNI, routes)                        │
│     ├─► Security Executor (initialize iptables chains)              │
│     ├─► Task Executor (initialize worker pool)                      │
│     └─► Health Monitor (start monitoring loops)                     │
│                                                                      │
│  3. Register with Engine                                             │
│     ├─► Send registration request with capabilities                 │
│     ├─► Receive agent ID and configuration                          │
│     └─► Start heartbeat loop                                        │
│                                                                      │
│  4. Start Task Processing                                            │
│     ├─► Open bidirectional gRPC stream                              │
│     ├─► Listen for tasks from Engine                                │
│     └─► Process tasks and report results                            │
│                                                                      │
│  5. Continuous Operation                                             │
│     ├─► Health monitoring and reporting                             │
│     ├─► Task execution                                              │
│     └─► State reconciliation                                        │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Task Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Task Execution Flow                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Engine                                                              │
│    │                                                                 │
│    │ 1. SendTask(task)                                              │
│    ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                     Task Executor                            │    │
│  │                                                              │    │
│  │  ┌─────────────┐    ┌──────────────┐    ┌────────────────┐ │    │
│  │  │   Receive   │───►│   Validate   │───►│    Route to    │ │    │
│  │  │    Task     │    │     Task     │    │   Component    │ │    │
│  │  └─────────────┘    └──────────────┘    └───────┬────────┘ │    │
│  │                                                  │          │    │
│  └──────────────────────────────────────────────────┼──────────┘    │
│                                                     │                │
│         ┌───────────────────┬───────────────────────┤                │
│         │                   │                       │                │
│         ▼                   ▼                       ▼                │
│  ┌─────────────┐    ┌─────────────┐         ┌─────────────┐         │
│  │  Container  │    │   Network   │         │  Security   │         │
│  │   Runtime   │    │    Node     │         │  Executor   │         │
│  └──────┬──────┘    └──────┬──────┘         └──────┬──────┘         │
│         │                  │                       │                 │
│         └──────────────────┴───────────────────────┘                 │
│                           │                                          │
│                           ▼                                          │
│                    ┌─────────────┐                                   │
│                    │   Report    │                                   │
│                    │   Result    │                                   │
│                    └──────┬──────┘                                   │
│                           │                                          │
│                           ▼                                          │
│                       Engine                                         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Related Documents

- [Engine and Agent Architecture Overview](../engine-agent-architecture-design.md)
- [Engine Components](../engine/README.md)
- [VPC Module](../../pkg/vpc/README.md)
