# [MED-CONS-001] Naming Inconsistency: NodeName vs AgentName

**Status**: FIXED (2026-03-07) — Renamed `NodeName` → `AgentName` in Go structs, `node_name` → `agent_name` in YAML config, `--node-name` → `--agent-name` in CLI flags. All layers now use "agent" consistently, matching the proto/gRPC convention.
**Severity**: Medium
**Category**: CONS
**Component**: pkg/agent, pkg/rpc/proto, pkg/types
**File(s)**: `pkg/agent/agent.go:37`, `pkg/rpc/proto/banyan/v1/engine.proto`, `pkg/types/config.go`

## Description

The same concept (the name of a worker machine) is referred to by different names across layers:

- Go structs: `NodeName` (Agent.Options, config structs)
- Proto/gRPC: `agent_name` (RegisterRequest, HeartbeatRequest)
- YAML config: `node_name`

## Evidence

- `pkg/agent/agent.go` Options struct: `NodeName string`
- `pkg/rpc/proto/banyan/v1/engine.proto`: `string agent_name = 1`
- `pkg/types/config.go`: `NodeName string \`yaml:"node_name"\``

## Impact

- Developers must mentally translate between "node" and "agent" depending on which layer they're working in
- New contributors cannot search for a single term to find all relevant code
- Increases cognitive load when debugging cross-layer issues

## Recommendation

Standardize on one term across the entire codebase. "Agent" is the user-facing term in documentation. Consider renaming internal `NodeName` to `AgentName` for consistency, or keep `NodeName` internally and document the mapping.
