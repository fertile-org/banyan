# [INFO-002] Token ID uses 128 bits instead of 256

**Severity**: Informational
**Component**: Authentication
**File(s)**: `pkg/engine/auth/jwt.go:216-222`

## Description

The `generateTokenID()` function creates token IDs with 128 bits of entropy, below the 256-bit standard in the secure defaults checklist.

## Evidence

```go
// jwt.go:216-222
func generateTokenID() (string, error) {
    b := make([]byte, 16)  // 128 bits
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return hex.EncodeToString(b), nil
}
```

**Checklist A4** requires: "Token entropy — crypto/rand, 256-bit."

## Impact

Minor — 128 bits is still computationally infeasible to brute force for practical purposes. This is informational because 256-bit is the standard and the current implementation is below it.

## Recommendation

Increase token ID to 32 bytes:
```go
b := make([]byte, 32)  // 256 bits
```

## Secure Default Consideration

**Checklist A4**: "Auth token minimum entropy (256-bit) — ENFORCE — Tokens must be cryptographically random and long enough to resist guessing."