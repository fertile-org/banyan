# [INFO-001] Connect API has no rate limiting

**Severity**: Informational
**Component**: Connect API
**File(s)**: `pkg/engine/connect_server.go:331-335`

## Description

The Connect HTTP API (dashboard server) has no rate limiting on authentication endpoints. The gRPC server has per-IP rate limiting (`loginLimiter`, `limiter`) but the HTTP API does not.

## Evidence

```go
// connect_server.go:331-335
var finalHandler http.Handler = handler
if srv.authDeps != nil {
    finalHandler = auth.ConnectAuthMiddleware(srv.authDeps)(finalHandler)
}
mux.Handle(path, corsMiddleware(finalHandler, allowedOrigins))
// No rate limiter middleware
```

Compare to gRPC server (`grpc_server.go:162`):
```go
loginLimiter = newRateLimiter(10, time.Minute)  // Rate limited
```

## Impact

An attacker hitting the dashboard login endpoint (port 9091) can attempt brute-force login without per-IP limits. The gRPC auth has rate limiting; the Connect API doesn't.

## Recommendation

Add rate limiting middleware to the Connect API:
```go
finalHandler = rateLimitMiddleware(finalHandler, 10, time.Minute)  // 10 req/min per IP
```

## Secure Default Consideration

**Checklist A7**: "Failed auth rate limiting — DEFAULT — Slow down brute-force attempts. Default to rate limiting after N failures."

The gRPC auth has this; the Connect API doesn't.