# [MED-003] JWT auth path allows user enumeration

**Severity**: Medium
**Responsibility**: Platform Issue
**Component**: Authentication
**File(s)**: `pkg/engine/auth/middleware.go:52-57`

## Description

The JWT authentication path returns different error messages for "user not found" vs "invalid token" — allowing enumeration of valid usernames.

## Evidence

**authenticateAndAuthorize in middleware.go:52-57**:
```go
if err != nil {
    return ctx, status.Errorf(codes.Unauthenticated,
        "user account not found")
}
```

Compare to `grpc_handlers_auth.go:37-41` (login handler) which correctly uses the same error:
```go
user, err := s.authDeps.Users.Get(ctx, req.Username)
if err != nil {
    // Same error for user-not-found and wrong-password (prevent enumeration)
    s.logLoginEvent(peerIP, req.Username, false, "invalid credentials")
    return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
}
```

The login handler prevents enumeration; the JWT middleware does not.

## Impact

**Who can exploit**: Remote attacker trying to find valid usernames.

**What they gain**: Build a list of valid usernames for targeted attacks.

**Blast radius**: Low — enumeration is the first step, not the attack itself.

## Recommendation

Use the same error message for both cases in the JWT middleware:
```go
if err != nil {
    return ctx, status.Errorf(codes.Unauthenticated,
        "invalid credentials")
}
```

Log the detailed reason server-side for debugging, but return only "invalid credentials" to the client.

## Secure Default Consideration

**Checklist E2**: "Auth failures return same error regardless of cause — ENFORCE — Don't distinguish 'invalid token' from 'token expired' from 'no token' in the response. This prevents enumeration."

The login path follows E2; the JWT auth path does not.