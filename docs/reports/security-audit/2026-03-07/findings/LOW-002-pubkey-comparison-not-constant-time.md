# [LOW-002] Public Key Validation Not Constant-Time

**Severity**: Low
**Status**: FIXED
**Responsibility**: Mitigation Gap
**Component**: Authentication
**File(s)**:
- `pkg/rpc/auth.go:64-68` (`v.AllowedKeys[publicKey]` — map lookup)

## Description

`PublicKeyValidator.Validate()` uses a Go map lookup to check public keys. Map lookups are not constant-time and could theoretically leak timing information about whether partial key prefixes match whitelist entries.

In contrast, session token validation at line 146 correctly uses `subtle.ConstantTimeCompare`.

Mitigating factor: public keys are public by nature (they are shared openly in WireGuard configurations), so learning whether a key is whitelisted has limited value compared to learning a secret token.

## Recommendation

For consistency, iterate over allowed keys using `subtle.ConstantTimeCompare`. Low priority given that public keys are not secrets.

## Fix

The custom public key authentication mechanism has been removed entirely. WireGuard now handles identity verification at the tunnel layer, so there is no public key comparison code remaining in the application.
