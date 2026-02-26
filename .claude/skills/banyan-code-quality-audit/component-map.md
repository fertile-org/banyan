# Banyan Component Map

This document defines how Banyan's packages connect. It is the source of truth for integration integrity checks. If a package exists but isn't wired according to this map, that's either a finding or this map needs updating.

**Update this document** when packages are added, removed, or rewired.

## Package Overview

```
cmd/
├── banyan-engine/     Entry point for the engine binary
│   └── cmd/           CLI commands (init, start, stop)
├── banyan-agent/      Entry point for the agent binary
│   └── cmd/           CLI commands (init, start, stop)
└── banyan-cli/        Entry point for the user CLI
    └── cmd/           CLI commands (init, deploy, down, status, logs, auth)

pkg/
├── engine/            Engine core: gRPC server, scheduling, orchestration loop
├── agent/             Agent core: task execution, container lifecycle, engine client
├── rpc/               Auth logic, gRPC interceptors, protobuf definitions
│   └── proto/         Proto source files and generated Go code
├── storage/           etcd client, store interface, memory store (for tests)
├── vpc/               VPC overlay networking (Flannel), DNS, security rules
├── types/             Shared types: manifest, config, records, helpers
└── shared/            Domain entities (AUDIT: may be unused)

website/
└── src/content/docs/  User-facing documentation (Starlight/Astro)
```

## Expected Wiring

This section defines which packages must be imported and initialized by which binaries. "Expected" means "if the docs claim this feature, this wiring must exist."

### Engine Binary (`cmd/banyan-engine`)

```
cmd/banyan-engine/cmd/engine.go → runEngineStart()
  │
  ├─ pkg/engine       → engine.New(options)
  │   ├─ pkg/storage  → storage.NewStoreWithOptions()     [etcd connection]
  │   ├─ pkg/vpc      → vpc.InitializeNetwork()           [overlay network setup]
  │   ├─ (internal)   → startRegistry()                   [OCI image registry]
  │   ├─ (internal)   → startEngineGRPC()                 [gRPC server]
  │   │   └─ pkg/rpc  → rpc.NewAuthInterceptor()          [auth middleware]
  │   └─ (internal)   → engineLoop()                      [orchestration ticker]
  │
  └─ pkg/types        → types.LoadConfig(), types.SaveConfig()
```

**Verification points:**
- `storage.NewStoreWithOptions()` — must be called with etcd endpoints from config
- `vpc.InitializeNetwork()` — must be called before agents register
- `startRegistry()` — must start HTTP server on configured port
- `startEngineGRPC()` — must register `EngineServiceServer` with auth interceptors
- `engineLoop()` — must be started as goroutine

### Agent Binary (`cmd/banyan-agent`)

```
cmd/banyan-agent/cmd/agent.go → runAgentStart()
  │
  ├─ pkg/agent        → agent.New(options), agent.Run(ctx)
  │   ├─ pkg/agent    → NewEngineClient()                 [gRPC client to engine]
  │   ├─ (internal)   → waitForEngineGRPC()               [readiness check]
  │   ├─ (internal)   → client.Register()                 [node registration]
  │   ├─ (internal)   → agentLoop()                       [task polling]
  │   ├─ (internal)   → agentHeartbeat()                  [heartbeat ticker]
  │   ├─ (internal)   → containerHealthLoop()             [health monitoring]
  │   └─ (internal)   → startAgentGRPC()                  [log streaming server]
  │       └─ pkg/rpc  → session token auth interceptor
  │
  └─ pkg/types        → types.LoadConfig()
```

**Verification points:**
- `NewEngineClient()` — must connect to engine gRPC endpoint from config
- `client.Register()` — must send node name, API address, session token
- All three background loops must start as goroutines with context cancellation
- `startAgentGRPC()` — must register `AgentServiceServer` with session token auth

### CLI Binary (`cmd/banyan-cli`)

```
cmd/banyan-cli/cmd/root.go → rootCmd with subcommands
  │
  ├─ init.go     → runInit()      [password exchange → token storage]
  ├─ deploy.go   → runDeploy()    [parse manifest → Deploy RPC]
  ├─ down.go     → runDown()      [Down RPC]
  ├─ status.go   → runStatus()    [GetStatus RPC]
  ├─ logs.go     → runLogs()      [GetLogs RPC → agent StreamLogs]
  └─ auth.go     → runAuth()      [re-authenticate → new token]

  Each command:
  ├─ pkg/agent   → agent.NewEngineClient() or NewEngineClientWithPassword()
  ├─ pkg/types   → types.LoadConfig(), types.Manifest, types.SaveConfig()
  └─ pkg/rpc     → (indirect, via generated protobuf stubs)
```

**Verification points:**
- Every CLI command listed in `reference/cli.md` must have a corresponding file and handler
- Every CLI command must load config, create client, make RPC call, handle response
- Deploy must validate manifest before sending

## gRPC Contract

### EngineService (engine.proto)

Every RPC defined in the proto MUST have a handler in `pkg/engine/grpc_server.go`:

| RPC | Purpose | Called By |
|-----|---------|----------|
| Register | Agent joins cluster | Agent startup |
| Heartbeat | Agent alive signal | Agent heartbeat loop |
| PollTasks | Agent fetches work | Agent task loop |
| ReportTaskResult | Agent reports task completion | Agent after container op |
| ReportContainerHealth | Agent reports container status | Agent health loop |
| Deploy | User deploys manifest | CLI `deploy` command |
| Down | User tears down deployment | CLI `down` command |
| GetStatus | User checks cluster state | CLI `status` command |
| GetLogs | User reads container logs | CLI `logs` command |
| GetInfo | User gets cluster info | CLI (if exposed) |
| Health | Readiness check | Agent startup |
| ExchangeToken | Password → token auth | CLI `init` and `auth` commands |

**Audit check**: If a new RPC is added to the proto, it must have a handler AND be reachable through CLI or agent code.

### AgentService (agent.proto)

| RPC | Purpose | Called By |
|-----|---------|----------|
| StreamLogs | Stream container logs from agent | Engine on behalf of CLI |

## Doc-to-Code Mapping

What the docs claim and where the code must be:

| Doc Claim | Doc Location | Implementation Must Be |
|-----------|-------------|----------------------|
| Docker Compose syntax | index.mdx, README | `pkg/types/manifest.go` — manifest parsing |
| Three binaries | index.mdx, README | `cmd/banyan-{engine,agent,cli}/` — all three must build and run |
| Built-in image registry | index.mdx, README | `pkg/engine/` — registry started in engine startup |
| Containers talk across servers | index.mdx, README | `pkg/vpc/` — initialized in engine, agents use overlay |
| DNS automatically | index.mdx | `pkg/vpc/dns/` or Flannel DNS — must resolve service names |
| Monitor from terminal | index.mdx, README | `cmd/banyan-cli/cmd/status.go`, `logs.go` — commands work |
| Terminal dashboard (coming soon) | index.mdx, README | roadmap.md only — must NOT be documented as working |
| Open source Apache 2.0 | index.mdx, README | LICENSE file exists |
| Prometheus-compatible | index.mdx architecture diagram | `/metrics` endpoint — must exist if claimed |
| Blue-green redeployment | roadmap.md (Done) | Deploy logic must support zero-downtime update |
| Per-service deployment | roadmap.md (Done) | Deploy RPC must accept partial manifests |
| Authentication | guides/authentication.md | `pkg/rpc/auth.go` — password and token validation |
| Multi-node | guides/multi-node.md | Agent registration, task scheduling to multiple agents |

**Audit rule**: For every "Done" item on the roadmap and every feature claim in the docs, trace the implementation from user action (CLI command or manifest field) through to actual execution.

## Known Problem Packages

Packages that have had integration issues in the past. Pay extra attention during audits:

| Package | Historical Issue | What to Check |
|---------|-----------------|---------------|
| `pkg/vpc/` | Was implemented but not integrated into engine/agent startup | Verify `vpc.InitializeNetwork()` is called in engine startup path |
| `pkg/shared/` | May contain unused domain entities | Verify it's imported by at least one production (non-test) file |

**Add to this table** when new integration issues are discovered and fixed.

## Import Dependency Graph

Expected production import chains (test imports excluded):

```
cmd/banyan-engine/cmd/
  └─ pkg/engine
       ├─ pkg/storage
       ├─ pkg/vpc
       ├─ pkg/rpc
       └─ pkg/types

cmd/banyan-agent/cmd/
  └─ pkg/agent
       ├─ pkg/rpc
       └─ pkg/types

cmd/banyan-cli/cmd/
  ├─ pkg/agent      (for EngineClient)
  ├─ pkg/rpc        (indirect, via generated code)
  └─ pkg/types
```

**Audit check**: Every `pkg/` directory must appear in at least one of these chains. If a package exists under `pkg/` but doesn't appear in any production import chain, it's either dead code or a missing integration.

## How to Update This Map

When Banyan adds a new component:

1. Add the package to the Package Overview
2. Add its expected wiring to the relevant binary section
3. Add any new gRPC RPCs to the gRPC Contract
4. Add doc claims to the Doc-to-Code Mapping
5. Update the Import Dependency Graph
6. Run the audit to verify the new wiring is in place
