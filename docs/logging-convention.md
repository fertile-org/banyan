# Banyan Logging Convention

## Overview

This document defines the logging standards for all Banyan components (Engine, Agent, VPC, CLI). Consistent logging is critical for debugging, monitoring, and maintaining the system in production.

## Core Principles

1. **Structured Logging** - Use JSON format for machine readability
2. **Contextual Information** - Include component, action, and relevant IDs
3. **Actionable Messages** - Suggest fixes when possible
4. **Performance Aware** - DEBUG logs should be minimal in production

## Log Levels

### INFO - User Actions & State Changes
User-initiated actions and significant state changes.

```
[INFO] Service deployed: web (3 instances)
[INFO] Configuration loaded: /etc/banyan/config.yaml
[INFO] Agent connected: agent-host-1 (10.1.1.1)
```

### DEBUG - Implementation Details
Low-level operations for troubleshooting. Should be disabled in production.

```
[DEBUG] Container network namespace created: /var/run/netns/web-001
[DEBUG] Health check response: web-001 returned 200 OK in 45ms
[DEBUG] State sync: received 15 updates from leader
```

### WARN - Recoverable Issues
Problems that don't stop operation but need attention.

```
[WARN] Retry attempt 2/5: connecting to etcd cluster
[WARN] Resource usage high: CPU at 85%
[WARN] Deprecated config option: use 'allow' instead of 'allow_from'
```

### ERROR - Operation Failures
Operations that failed and require intervention.

```
[ERROR] Service start failed: port 80 already in use
[ERROR] State store unreachable: etcd timeout after 30s
[ERROR] Invalid configuration: service 'web' missing image
```

### FATAL - System Cannot Continue
Critical failures that cause the process to terminate. Always followed by exit.

```
[FATAL] Cannot bind to port 8080: address already in use (exiting)
[FATAL] Configuration file not found: /etc/banyan/config.yaml (exiting)
[FATAL] Incompatible database schema version: expected v3, found v1 (exiting)
```

## Structured Log Format

All components should output logs in JSON format for easy parsing:

```json
{
  "timestamp": "2024-01-10T10:30:00Z",
  "level": "ERROR",
  "component": "engine.deploy",
  "request_id": "req-abc123",
  "host": "host-1",
  "service": "web",
  "error": "port 80 already in use",
  "suggestion": "Check for existing processes on port 80"
}
```

## Component Identifiers

Each component uses a specific prefix for clarity:

| Component | Prefix | Example |
|-----------|--------|---------|
| Engine | `engine` | `engine.deploy`, `engine.scheduler` |
| Agent | `agent` | `agent.docker`, `agent.health` |
| VPC | `vpc` | `vpc.network`, `vpc.security` |
| CLI | `cli` | `cli.init`, `cli.deploy` |
| State | `state` | `state.etcd`, `state.sync` |

## Best Practices

1. **Include Request IDs** - Track operations across components
2. **Log at Boundaries** - When entering/exiting major operations
3. **Avoid Sensitive Data** - Never log passwords, keys, or tokens
4. **Use Appropriate Levels**:
   - **INFO**: Normal operations, state changes, successful actions
   - **DEBUG**: Detailed execution flow, only in development/troubleshooting
   - **WARN**: Recoverable issues, retries, degraded performance, deprecations
   - **ERROR**: Operation failures requiring intervention or investigation
   - **FATAL**: Unrecoverable errors causing process termination
5. **Add Context** - Include service names, host IDs, etc.
6. **Suggest Solutions** - Help users fix problems

## Required Fields

Every log entry MUST include:
- `timestamp` - ISO 8601 format
- `level` - INFO/DEBUG/WARN/ERROR/FATAL
- `component` - Component and subcomponent
- `request_id` - Unique ID for tracking across components
- `host` - Hostname where log originated

## Log Output Format

While logs are structured as JSON internally, they should be formatted for human readability when displayed:

```bash
# What developers see in terminal
2024-01-10T10:30:00Z [INFO] engine.deploy: Deployment started app=app-v1.2.3 request_id=req-abc123

# What gets stored/sent to aggregators (JSON)
{"timestamp":"2024-01-10T10:30:00Z","level":"INFO","component":"engine.deploy","request_id":"req-abc123","host":"host-1","msg":"Deployment started","app":"app-v1.2.3"}
```

## Examples by Component (Human-Readable Output)

### Engine Logs
```
2024-01-10T10:30:00Z [INFO] engine.deploy: Deployment started app=app-v1.2.3 request_id=req-abc123
2024-01-10T10:30:01Z [INFO] engine.scheduler: Service scheduled service=web hosts=[host-1,host-2,host-3] request_id=req-abc123
2024-01-10T10:30:02Z [ERROR] engine.deploy: Deployment failed - insufficient resources (need 4GB RAM, have 2GB) request_id=req-abc123
```

### Agent Logs
```
2024-01-10T10:30:00Z [INFO] agent.docker: Container started container=web-001 docker_id=d3f4a5b6 request_id=req-abc123
2024-01-10T10:31:00Z [WARN] agent.health: Health check failed container=web-001 status=503 retry=1/3 request_id=req-abc123
2024-01-10T10:32:00Z [ERROR] agent.docker: Container crashed container=web-001 exit_code=137 reason="OOM killed" request_id=req-abc123
```

### VPC Logs
```
2024-01-10T10:30:00Z [INFO] vpc.network: Network created network=vpc-prod cidr=10.0.0.0/16 request_id=req-def456
2024-01-10T10:30:01Z [DEBUG] vpc.security: Security rule added from=10.0.1.5 to=10.0.2.10:5432 action=ALLOW request_id=req-def456
2024-01-10T10:30:02Z [WARN] vpc.security: Connection blocked from=web to=redis:6379 reason="no allow rule" suggestion="Add allow rule for service:web" request_id=req-def456
```