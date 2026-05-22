# TODOS

Deferred work items from M13 Auth review. Each item includes context for
someone picking it up in 3 months.

## P0 — TLS Certificate Rotation (critical — system fails after 1 year)

**What:** Server TLS certificates (`~/.banyan/tls/server.crt`) expire after 1 year
(`serverValidityYears = 1` in `pkg/engine/auth/tls.go:23`). When expired, all
CLI→Engine and Agent→Engine communication fails with TLS handshake errors.

**Why:** This is a critical operational issue, not a missing feature. A production
cluster running for 12+ months becomes completely inoperable. Recovery requires
regenerating certs (changing CA fingerprint) and re-registering all agents.

**How to start:** Add a background goroutine in the engine that:
1. Checks cert expiry on startup and periodically (e.g., daily)
2. Renews certs when `NotAfter - 30 days < now` (30-day warning window)
3. Stores renewal timestamp to avoid duplicate regeneration
4. On CA renewal: existing agents need the new CA fingerprint to connect

For v1: warn via logs at startup if cert expires in <90 days.
For v2: automate rotation with agent notification/distribution.

**Depends on:** M13 auth (cert generation exists in `tls.go`).

**Effort:** M (human: ~1 week / CC: ~2 hours)

---

## P0 — ABAC Attribute Evaluation (interface ready, implementation pending)

**What:** The `Authorizer` interface (`pkg/engine/auth/types.go:37`) is ABAC-shaped
(`subject, resource, action`) but `RoleAuthorizer` (`authorizer.go`) implements
static RBAC — it ignores `ctx` and does a simple role→permission map lookup.

**Why:** True ABAC would evaluate attributes from context (e.g., `environment=production`,
`time_of_day`, `source_ip`) enabling fine-grained policies without new roles.

**How to start:** Refactor `RoleAuthorizer.Authorize()` to extract attributes from
`ctx` (via `UserFromContext`, env tags, etc.) and evaluate them before falling back
to the static permission map. Keep `RoleAuthorizer` as the simple UX layer or
create a separate `ABACAuthorizer` that wraps it.

**Depends on:** M13 auth (done — interface in place).

**Effort:** M (human: ~1 week / CC: ~1-2 hours)

---

## P1 — Logout Leaks Refresh Tokens (store bloat)

**What:** `banyan logout` (`runLogout` in `cmd/banyan-cli/cmd/login.go`) "revokes" the
session by calling the `RefreshToken` RPC. But that RPC *rotates* — `ValidateRefreshToken`
(`pkg/engine/auth/jwt.go`) revokes the old token AND issues a new pair. Logout never
returns that new pair to anyone, so a fresh `RefreshTokenRecord` is persisted to etcd
and orphaned. It then lingers for the full 7-day refresh TTL (`CleanupExpiredTokens`
only reaps *expired* records).

**Why:** Not a security hole — the old token *is* invalidated by the rotation, so a
logged-out session cannot refresh. But every logout leaves dead state in etcd. A
long-lived cluster with frequent logins/logouts slowly accumulates orphaned token
records. `JWTManager.RevokeRefreshToken` (`jwt.go`, comment says "used by logout")
already exists and does the right thing — it is simply not exposed over RPC.

**How to start:**
1. Add a `Logout` (or `RevokeToken`) RPC to `pkg/rpc/proto/banyan/v1/engine.proto`
   that takes a refresh token and calls `JWTManager.RevokeRefreshToken` — no rotation.
2. Regenerate protobuf, add the handler in `pkg/engine/grpc_handlers_auth.go`.
3. Point `runLogout` at the new RPC instead of `RefreshToken`.

**Depends on:** M13 auth (done — `RevokeRefreshToken` already implemented).

**Effort:** S (human: ~1 day / CC: ~30-45 min)

---

## P1 — SSO/OIDC Support

**What:** Add `banyan login --sso` that opens a browser for OAuth2/OIDC authentication
(Google, Okta, Azure AD). Maps OIDC claims to Banyan roles.

**Why:** Companies want engineers to use their existing identity provider, not manage
separate Banyan passwords. This is the #1 feature companies will ask for after basic
auth ships.

**How to start:** The `Authorizer` interface is already in place. Add an `OIDCProvider`
that implements token exchange (OIDC id_token -> Banyan JWT). The CLI opens a browser,
receives the callback on a local port, exchanges the code for tokens.

**Depends on:** M13 auth (JWT infrastructure, UserStore, Authorizer interface).

**Effort:** L (human: ~2 weeks / CC: ~2-3 hours)

---

## P2 — API Keys for CI/CD

**What:** Long-lived tokens for automation pipelines (GitHub Actions, GitLab CI).
`banyan api-key create --role deployer --name "CI pipeline"` generates a key that
works as a Bearer token.

**Why:** CI/CD pipelines can't do interactive login. They need a pre-generated token
with scoped permissions. Without this, teams can't automate deployments.

**How to start:** New token type stored hashed in etcd at `auth/api-keys/`. The auth
middleware already checks Bearer tokens. API keys bypass refresh (they don't expire
but are revocable). Add `banyan api-key create/list/revoke` CLI commands.

**Depends on:** M13 auth.

**Effort:** M (human: ~1 week / CC: ~1-2 hours)

---

## P2 — Auth Audit Log

**What:** Log every authorization decision (who did what, when, from where) to a
queryable store. Decorator on the `Authorizer` interface.

**Why:** Compliance checkbox. Admins need to answer "who deployed to production at 3am?"

**How to start:** Wrap `RoleAuthorizer` with an `AuditAuthorizer` that logs every
`Authorize()` call to etcd or a file. The `Authorizer` interface makes this a clean
decorator pattern. Add `banyan audit list` CLI command.

**Depends on:** M13 auth.

**Effort:** S (human: ~2 days / CC: ~30 min)
