# MED-004: DNS server defaults to 0.0.0.0:53 in struct

**Severity**: Medium
**Responsibility**: Default Issue
**Component**: VPC — DNS Server
**File(s)**: `pkg/vpc/dns/server.go:37`

## Description

The `DefaultServerConfig()` function returns `BindAddr: "0.0.0.0:53"`. While the agent overrides this with the gateway IP, the unsafe default could affect future callers.

## Impact

Mitigated in practice — agents always override with internal IP. The risk is a future code path that uses the default.

## Recommendation

Change default to `"127.0.0.1:53"` or empty string (force callers to specify).

## Secure Default Consideration

Default should bind to loopback only. Callers that need a different address should set it explicitly.
