# [MED-SMELL-002] Auth RPC handlers lack dedicated tests

**Severity**: Medium
**Category**: SMELL
**Component**: pkg/engine/grpc_handlers_auth.go
**File(s)**: `pkg/engine/grpc_handlers_auth.go:1-256`

## Description

The auth RPC handlers (Login, ListUsers, CreateUser, DeleteUser, ChangePassword, UpdateUserRole, RefreshToken) are implemented in `grpc_handlers_auth.go` but have no dedicated test coverage. These handlers are critical for security as they handle authentication and user management.

## Evidence

**Test coverage analysis**:
- `grpc_handlers_auth.go` (256 lines) - NO dedicated test file
- `Login` is only tested indirectly via other test paths
- `ListUsers`, `CreateUser`, `DeleteUser`, `ChangePassword`, `UpdateUserRole` have NO test coverage
- `RefreshToken` has NO test coverage

**Related auth tests exist but cover different layers**:
- `pkg/rpc/auth_test.go` - tests RPC-level auth, not the handlers
- `pkg/engine/auth/` - tests JWT, store, roles, TLS, middleware, authorizer
- But NOT the actual gRPC handlers in `grpc_handlers_auth.go`

## Impact

**Security impact**: Auth handlers are security-critical. Without tests, changes to these handlers could introduce authentication bypasses, privilege escalation, or token management bugs that go undetected.

**Reliability impact**: User management operations (create, delete, role changes) have no automated verification of correct behavior.

## Recommendation

1. **Add test file**: Create `pkg/engine/grpc_handlers_auth_test.go`
2. **Cover all auth handlers**:
   - `TestLogin` - valid/invalid credentials, token generation
   - `TestRefreshToken` - token rotation
   - `TestCreateUser` - validation, role assignment
   - `TestListUsers` - pagination, role filtering
   - `TestDeleteUser` - cleanup, protection against self-delete
   - `TestChangePassword` - validation, admin vs user flows
   - `TestUpdateUserRole` - permission changes

3. **Test edge cases**:
   - Invalid credentials
   - Expired tokens
   - Missing required fields
   - Unauthorized access attempts