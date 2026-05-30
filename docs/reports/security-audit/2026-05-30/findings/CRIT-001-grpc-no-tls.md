# [CRIT-001] gRPC server has no TLS — tokens and manifests transmitted in plaintext

**Severity**: Critical
**Responsibility**: Platform Issue
**Component**: gRPC Server, Authentication
**File(s)**: `pkg/engine/grpc_server.go:185`, `pkg/engine/connect_server.go:339`, `cmd/banyan-cli/cmd/client.go:29-32`

## Description

The gRPC server is created with **no TLS configuration**. All gRPC traffic — including JWT tokens, deployment manifests (which may contain secrets in environment variables), and agent communications — flows over plain TCP.

## Evidence

**gRPC server has no TLS** (`pkg/engine/grpc_server.go:185`):
```go
srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(unaryInterceptors...),
    grpc.ChainStreamInterceptor(streamInterceptors...),
    // No grpc.Creds option — plaintext TCP
)
```

**Connect API uses plaintext HTTP/2** (`pkg/engine/connect_server.go:339`):
```go
httpServer := &http.Server{
    Handler: h2c.NewHandler(mux, &http2.Server{}),
    // h2c = HTTP/2 over plain TCP, no TLS
}
```

**CLI unconditionally uses insecure credentials** (`cmd/banyan-cli/cmd/client.go:29-32`):
```go
dialOpts := []grpc.DialOption{
    grpc.WithTransportCredentials(insecure.NewCredentials()),
}
```

**TLS infrastructure exists but is never used**:
- `auth.GenerateTLSBundle()` creates certificates (tls.go:42)
- `auth.LoadServerTLSConfig(dir)` loads the config (tls.go:188)
- But neither is called in server startup — certs exist on disk but no server uses them

## Impact

**Who can exploit**: Network attacker on the same VPC, adjacent server, or internet if ports are exposed.

**What they gain**: 
- JWT tokens transmitted in plaintext — impersonate any user or agent
- Deployment manifests with environment variables — steal secrets
- Agent registration data — map cluster structure

**Blast radius**: Entire cluster — network-level access to the engine gives full control.

## Recommendation

1. In `startEngineGRPC()` (grpc_server.go:185), apply TLS credentials:
```go
tlsConfig, err := auth.LoadServerTLSConfig(opts.TLSCertDir)
if err != nil {
    return nil, fmt.Errorf("load TLS config: %w", err)
}
creds := grpc.Creds(credentials.NewTLS(tlsConfig))
srv := grpc.NewServer(
    grpc.Creds(creds),
    grpc.ChainUnaryInterceptor(unaryInterceptors...),
    // ...
)
```

2. In `startConnectAPI()` (connect_server.go), use `http.Server` with TLS config instead of `h2c.NewHandler()`.

3. In CLI client, detect TLS availability and warn if connecting without encryption:
```go
if !hasWireGuardTunnel() {
    slog.Warn("Connecting without TLS — gRPC traffic may be unencrypted")
}
```

## Secure Default Consideration

**Checklist T1**: "TLS on Engine gRPC server — DEFAULT — All gRPC traffic carries tokens and manifests. Plaintext means network eavesdroppers see everything."

This is a **Platform Issue** — the code has no TLS path, not even a broken one. The TLS infrastructure exists but zero code paths connect it to the servers.