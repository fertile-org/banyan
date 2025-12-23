# Engine Component Designs

This folder contains detailed design documents for each Engine component.

## Overview

The Engine is the **control plane** of Banyan. It orchestrates deployments, manages state, coordinates agents, and integrates with the VPC networking layer.

## Architecture Pattern

All components follow **Clean Architecture** with **Hexagonal Architecture** (Ports and Adapters):

```
┌─────────────────────────────────────────────────────────┐
│                 Driving Adapters                         │
│  (gRPC handlers, CLI handlers)                          │
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
│  (etcd, gRPC clients, external services)                │
└─────────────────────────────────────────────────────────┘
```

## Components

| Component | Description | Document |
|-----------|-------------|----------|
| **Orchestrator** | Manages deployment workflows and pipelines | [orchestrator.md](./orchestrator.md) |
| **State Manager** | Handles desired vs actual state reconciliation | [state-manager.md](./state-manager.md) |
| **Agent Registry** | Manages agent registration and selection | [agent-registry.md](./agent-registry.md) |
| **Plugin Manager** | Lifecycle plugins (Type 2) | [plugin-manager.md](./plugin-manager.md) |
| **VPC Coordinator** | Bridges to VPC managers for network control | [vpc-coordinator.md](./vpc-coordinator.md) |
| **Compose Parser** | Parses docker-compose.yaml and banyan.yaml files | [compose-parser.md](./compose-parser.md) |

## Directory Structure

```
pkg/engine/
├── engine.go                  # Main Engine implementation
├── types.go                   # Shared types
├── orchestrator/
│   ├── domain/
│   │   ├── entities.go
│   │   └── value_objects.go
│   ├── ports/
│   │   ├── inbound.go
│   │   └── outbound.go
│   ├── usecases/
│   │   └── deploy.go
│   └── adapters/
│       ├── grpc_handler.go
│       └── etcd_repository.go
├── state/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── registry/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── plugin/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── parser/
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   ├── adapters/
│   └── errors/
└── vpc/
    ├── domain/
    ├── ports/
    ├── usecases/
    └── adapters/
```

## Component Interactions

```
┌─────────────────────────────────────────────────────────────────┐
│                           Engine                                 │
│                                                                  │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐    │
│  │Compose Parser│────►│ Orchestrator │────►│ State Manager│    │
│  │ (yaml files) │     └──────┬───────┘     └──────┬───────┘    │
│  └──────────────┘            │                    │             │
│                              │                    ▼             │
│                              │             ┌──────────────┐    │
│                              │             │  Reconciler  │    │
│                              ▼             └──────────────┘    │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐    │
│  │Agent Registry│────►│   Scheduler  │────►│  Dispatcher  │    │
│  └──────────────┘     └──────────────┘     └──────┬───────┘    │
│                                                    │             │
│  ┌──────────────┐     ┌──────────────┐            │             │
│  │Plugin Manager│────►│ Lifecycle    │            │             │
│  │  (Type 2)    │     │   Hooks      │            │             │
│  └──────────────┘     └──────────────┘            │             │
│                                                    │             │
│  ┌──────────────────────────────────────┐         │             │
│  │         VPC Coordinator              │         │             │
│  │  ┌─────────┐ ┌──────┐ ┌──────────┐  │         │             │
│  │  │ Network │ │ IPAM │ │ Security │  │         │             │
│  │  │   Mgr   │ │  Mgr │ │    Mgr   │  │         │             │
│  │  └─────────┘ └──────┘ └──────────┘  │         │             │
│  └──────────────────────────────────────┘         │             │
└───────────────────────────────────────────────────┼─────────────┘
                                                    │
                                              gRPC  │
                                                    ▼
                                              [ Agents ]
```

## Related Documents

- [Engine and Agent Architecture Overview](../engine-agent-architecture-design.md)
- [Agent Components](../agent/README.md)
- [VPC Module](../../pkg/vpc/README.md)
