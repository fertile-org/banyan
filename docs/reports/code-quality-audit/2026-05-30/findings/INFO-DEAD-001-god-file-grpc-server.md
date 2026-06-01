# [INFO-DEAD-001] God file: grpc_server.go at ~2000+ lines

**Severity**: Informational
**Category**: DEAD
**Component**: pkg/engine/grpc_server.go
**File(s)**: `pkg/engine/grpc_server.go:1-~2000`

## Description

The file `grpc_server.go` has grown to approximately 2000+ lines (reported as 2128 lines in the 2026-03-27 audit). While it remains manageable, it's approaching the threshold where maintainability becomes a concern.

## Context

The previous audit (2026-03-27) noted this as [INFO-SMELL-001](findings/INFO-SMELL-001-god-file-grpc-server.md). The file has likely grown since then as new RPCs were added.

**Note**: The handlers have been split into multiple files (`grpc_handlers_agent.go`, `grpc_handlers_cli.go`, `grpc_handlers_dashboard.go`, `grpc_handlers_web.go`, `grpc_handlers_secrets.go`, `grpc_handlers_auth.go`), which is good. But the main `grpc_server.go` file still contains the server struct definition and initialization logic.

## Impact

**Developer impact**: Large files are harder to navigate, understand, and modify. The risk of introducing bugs increases with file size.

**Maintenance impact**: Code reviews become more difficult when files are large.

## Recommendation

1. **Monitor size**: Continue to track this metric. If it exceeds 2500 lines, consider further splitting.
2. **Keep handler logic separate**: The current approach of putting handler code in separate files is correct — don't merge them back.
3. **Consider interface extraction**: If the `Server` struct is becoming complex, consider extracting some of its dependencies into interfaces for easier testing and composition.