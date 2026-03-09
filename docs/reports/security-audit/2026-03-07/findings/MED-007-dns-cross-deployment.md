# [MED-007] DNS Resolves Services Across All Deployments

**Severity**: Medium
**Status**: FIXED
**Responsibility**: Platform Issue
**Component**: VPC DNS
**File(s)**:
- `pkg/agent/vpc_networking.go:353-391` (`reconcileDNS`)
- `pkg/agent/agent.go:297-301` (local DNS registration)

## Description

`reconcileDNS()` iterates ALL backends from all deployments and registers them by `ServiceName + ".internal"`. There is no deployment or namespace scoping.

If deployment A has a service named `db` and deployment B also has a service named `db`, DNS returns IPs from both deployments. Any container can resolve any service name from any deployment.

## Impact

- Containers can discover and access services from other deployments
- Service name collisions across deployments cause unpredictable DNS resolution
- Violates the principle of deployment isolation

## Recommendation

Namespace DNS by deployment: `<service>.<deployment>.internal`. Allow shortname resolution only within the same deployment.

## Fix

DNS is now namespaced by deployment. Services are registered as `<service>.<deployment>.internal` (FQDN). Short names (`<service>.internal`) are only registered when there is no cross-deployment conflict for that service name, preventing unintended cross-deployment resolution.
