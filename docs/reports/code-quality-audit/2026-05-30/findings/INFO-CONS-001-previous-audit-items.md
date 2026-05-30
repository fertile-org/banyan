# [INFO-CONS-001] Previous audit items still open

**Severity**: Informational
**Category**: CONS
**Component**: Multiple
**File(s)**: Various

## Description

Several issues from previous audits remain unresolved:

1. **God file (grpc_server.go)** - Reported in 2026-03-06 and 2026-03-27, still ~2000+ lines
2. **Agent/Node naming inconsistency** - [MED-CONS-001](findings/MED-CONS-001-agent-vs-node-naming.md) from 2026-03-27, still unresolved

## Impact

**Developer impact**: Accumulated tech debt. Each sprint that passes without fixing these increases the cost of eventual resolution.

**System impact**: Low - these are maintainability issues, not production bugs.

## Recommendation

1. **Prioritize the naming inconsistency fix**: It's a straightforward rename that will reduce cognitive load for all developers.
2. **Schedule god file review**: Set a milestone to address the grpc_server.go size before it becomes a critical issue.
3. **Track debt**: Add these to the project's tech debt backlog if not already there.