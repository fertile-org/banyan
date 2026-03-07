# [MED-BUILD-001] go mod tidy Would Change pkg/agent/go.mod

**Status**: FIXED (2026-03-06)
**Severity**: Medium
**Category**: BUILD
**Component**: pkg/agent
**File(s)**: `pkg/agent/go.mod`

## Description

Running `go mod tidy` on `pkg/agent` produces a diff: `github.com/coreos/go-iptables` should be a direct dependency (not indirect).

## Evidence

```diff
 require (
+	github.com/coreos/go-iptables v0.8.0
 	github.com/fertile-org/banyan/pkg/logging v0.0.0
 ...
 require (
-	github.com/coreos/go-iptables v0.8.0 // indirect
 	github.com/coreos/go-semver v0.3.1 // indirect
```

The package directly imports `github.com/coreos/go-iptables/iptables` in `vpc_networking.go` but lists it as indirect.

## Impact

- Misleading dependency classification
- Could cause issues with dependency management tooling

## Recommendation

Run `go mod tidy` in `pkg/agent/` to fix the dependency classification.
