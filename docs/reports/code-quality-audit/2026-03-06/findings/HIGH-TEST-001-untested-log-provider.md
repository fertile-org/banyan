# [HIGH-TEST-001] Untested Core Agent Component: log_provider.go

**Status**: FIXED (2026-03-06) — Added log_provider_test.go with cmdReadCloser and StreamLogs tests
**Severity**: High
**Category**: TEST
**Component**: pkg/agent
**File(s)**: `pkg/agent/log_provider.go`

## Description

`log_provider.go` implements `NerdctlLogProvider.StreamLogs()` and `cmdReadCloser.Close()` — the log streaming pipeline from containers to CLI users. This 61-line file has **zero test coverage**. No `log_provider_test.go` exists.

## Evidence

- File exists: `pkg/agent/log_provider.go` (61 lines)
- No corresponding test file
- No references to `NerdctlLogProvider` or `cmdReadCloser` in any test file
- Functions execute system commands (`nerdctl logs`) and manage OS processes

## Impact

- Log streaming is a user-visible feature (CLI `logs` command)
- Process cleanup in `cmdReadCloser.Close()` could leak processes on error
- No validation that nerdctl command construction is correct
- No testing of error paths (container not found, nerdctl not installed)

## Recommendation

Add `log_provider_test.go` with:
1. Test that `StreamLogs` constructs the correct nerdctl command
2. Test that `cmdReadCloser.Close()` properly kills the process
3. Test error handling when the container doesn't exist
