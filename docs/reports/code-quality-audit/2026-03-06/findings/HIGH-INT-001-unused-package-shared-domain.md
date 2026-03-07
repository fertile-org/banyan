# [HIGH-INT-001] Unused Package: pkg/shared/domain

**Status**: FIXED (2026-03-06) — Removed from go.work and deleted directory
**Severity**: High
**Category**: INT
**Component**: pkg/shared/domain
**File(s)**: `go.work:13`, `pkg/shared/domain/*.go`

## Description

The `pkg/shared/domain` package is listed in `go.work` and contains domain entities (errors, IDs, status types, values) with full test coverage. However, it is **not imported by any production code** in the entire codebase.

## Evidence

- `go.work:13` lists `./pkg/shared/domain` as a workspace module
- Package contains 5 source files: `errors.go`, `ids.go`, `status.go`, `values.go`, `events.go`
- Package contains 4 test files with passing tests
- Grep for `"github.com/fertile-org/banyan/pkg/shared/domain"` across all Go files returns **zero results**
- No `cmd/` or `pkg/` file imports this package

## Impact

- **Maintenance burden**: Tests run, code is maintained, but nothing uses it
- **Developer confusion**: New contributors may think these types are the canonical domain types, when `pkg/types/` is the actual source of truth
- **Codebase bloat**: ~500 lines of code and tests that serve no purpose

## Recommendation

Remove `pkg/shared/domain` from `go.work` and delete the directory. If any types are needed in the future, they should be added to `pkg/types/` following the established pattern.
