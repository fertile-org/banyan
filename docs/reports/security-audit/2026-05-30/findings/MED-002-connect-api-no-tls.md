# [MED-002] Connect API dashboard uses plaintext HTTP/2 (h2c)

**Severity**: Medium
**Responsibility**: Default Issue
**Component**: Connect API, gRPC Server
**File(s)**: `pkg/engine/connect_server.go:339`

## Description

The web dashboard HTTP API is served using `h2c` — HTTP/2 over plain TCP without TLS. This is the Connect API that serves the Banyan web dashboard.

## Evidence

```go
// pkg/engine/connect_server.go:331-339
var finalHandler http.Handler = handler
if srv.authDeps != nil {
    finalHandler = auth.ConnectAuthMiddleware(srv.authDeps)(finalHandler)
}
mux.Handle(path, corsMiddleware(finalHandler, allowedOrigins))

// Line 339 — plaintext HTTP/2
httpServer := &http.Server{
    Handler: h2c.NewHandler(mux, &http2.Server{}),
}
```

The `h2c` handler provides HTTP/2 without TLS — suitable for development but not for production where the dashboard may handle session tokens.

## Impact

**Who can exploit**: Network attacker who can reach the dashboard port (default 9091).

**What they gain**: 
- Steal session tokens from dashboard traffic
- Intercept dashboard API calls
- potentially impersonate users

**Blast radius**: Dashboard users — session token exposure.

## Recommendation

1. Use TLS for the dashboard server:
```go
cert, err := tls.LoadX509KeyPair(certFile, keyFile)
if err != nil {
    return err
}
httpServer := &http.Server{
    TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
    Handler:   h2c.NewHandler(mux, &http2.Server{}),
}
// Listen with TLS: httpServer.ListenAndServeTLS("", "")
```

2. Or document that the dashboard should only be accessed via localhost or through a TLS-terminating proxy.

## Secure Default Consideration

**Checklist T1**: "TLS on Engine gRPC server — DEFAULT — All gRPC traffic carries tokens and manifests."

The Connect API is not gRPC but it still handles authentication and should use TLS in production.