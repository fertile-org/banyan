# [MED-BUILD-001] pkg/agent go.mod Needs Tidy

**Severity**: Medium
**Category**: BUILD
**Component**: pkg/agent
**File(s)**: `pkg/agent/go.mod`

## Description

`go mod tidy` would change `pkg/agent/go.mod` — `github.com/coreos/go-iptables` should be a direct dependency (not indirect).

## Evidence

```diff
-	github.com/coreos/go-iptables v0.8.0 // indirect
+	github.com/coreos/go-iptables v0.8.0
```

The agent package directly imports `go-iptables` but it's listed as indirect.

## Impact

- **Build health**: Inaccurate dependency classification
- **CI**: Could cause issues with strict module checks

## Recommendation

Run `cd pkg/agent && go mod tidy`.
