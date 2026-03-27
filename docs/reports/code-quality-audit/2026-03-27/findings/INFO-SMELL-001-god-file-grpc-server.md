# INFO-SMELL-001: grpc_server.go at 2,128 lines

**Severity**: Informational
**Category**: SMELL
**Component**: pkg/engine/grpc_server.go
**File(s)**: `pkg/engine/grpc_server.go` (2,128 lines)

## Description

The main gRPC server file has grown to 2,128 lines with 17 RPC handlers plus interceptors, helpers, and proto conversion functions. It handles agent registration, task polling, deployment, scaling, secrets, status, logs, and dashboard.

## Impact

Not a bug. The file is well-organized with clear sections. However, it will continue growing as new RPCs are added. Consider splitting by responsibility (agent RPCs, CLI RPCs, secret RPCs, dashboard) when it exceeds ~2,500 lines.

## Recommendation

No action needed now. Monitor growth. Natural split points: secret handlers (~80 lines), dashboard handler (~150 lines), proto conversion functions (~100 lines).
