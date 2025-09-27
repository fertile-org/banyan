# Two-Plugin Architecture Design for Banyan

this document evaluates the two-plugin model and provides recommendations for implementation.

### Plugin Type 1: Service Plugins (Sidecar Pattern)
```yaml
services:
  web:
    plugins:
      - plugin_name: application_load_balancer  # Deploys alongside web service
        parameters:
          listener_port: 443
```

**Benefits**:
- **Simple deployment model** - Plugin deploys with the service
- **Service-scoped** - Each service gets its own plugin instance
- **Resource locality** - Plugin and service share network/resources OR Plugin have standalone placement (like ALB should only have 1 instance)

**Good for**: Load balancers, service mesh sidecars, monitoring agents, backup utilities

### Plugin Type 2: Lifecycle Plugins (Engine-level)
```
Core Engine Lifecycle:
Validate → Plan → Deploy → Verify → Destroy
     ↓       ↓       ↓        ↓        ↓
   Plugin  Plugin  Plugin   Plugin   Plugin
 (gRPC)   (gRPC)  (gRPC)   (gRPC)   (gRPC)
```

**Benefits**:
- **Process isolation** - Plugins can't crash the engine
- **Multi-language** - Any language supporting gRPC/protobuf
- **Independent scaling** - Plugins run on their own resources
- **Hook-based** - Clean event-driven architecture

**Good for**: Cloud provider integrations, infrastructure provisioning, compliance checks, deployment strategies

## Why This Design

### 1. Clear Separation of Concerns
- **Type 1**: Application-level functionality (service-specific)
- **Type 2**: Infrastructure-level functionality (deployment-wide)

### 2. MCP-inspired Architecture
Your MCP server comparison is spot-on! The independent process model provides:
- Fault isolation
- Language flexibility  
- Independent development/deployment cycles
- Clear communication boundaries

### 3. Example
```bash
# Core engine starts up
banyan start

# Discovers and starts lifecycle plugins
banyan plugin discover
banyan plugin start monitoring-hooks

# Plugins register for lifecycle events
monitoring-hooks → register(Deploy, Verify)

# During deployment
banyan deploy → triggers lifecycle → calls registered plugins via gRPC
```

## Potential Considerations

### Configuration Complexity
```yaml
# How do users configure both types?
services:
  web:
    # Type 1: Service plugins
    plugins:
      - name: alb
        config: {...}
    
# Type 2: Lifecycle plugins (global?)
plugins:
  lifecycle:
    - name: compliance-checker  
      hooks: [verify]
```

### State Management
- Type 1: State stored with service
- Type 2: State managed by core engine
- Need clear boundaries on who owns what state
