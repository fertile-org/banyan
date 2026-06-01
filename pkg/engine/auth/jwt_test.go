package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

func newTestJWTManager() *JWTManager {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	store := storage.NewMemoryStore()
	return NewJWTManager(key, store)
}

func TestCreateTokenPair_HappyPath(t *testing.T) {
	m := newTestJWTManager()
	access, refresh, err := m.CreateTokenPair("alice", RoleDeployer)
	if err != nil {
		t.Fatalf("CreateTokenPair: %v", err)
	}
	if access == "" {
		t.Error("access token should not be empty")
	}
	if refresh == "" {
		t.Error("refresh token should not be empty")
	}
	if access == refresh {
		t.Error("access and refresh tokens should be different")
	}
}

func TestValidateAccessToken_HappyPath(t *testing.T) {
	m := newTestJWTManager()
	access, _, err := m.CreateTokenPair("alice", RoleDeployer)
	if err != nil {
		t.Fatalf("CreateTokenPair: %v", err)
	}

	claims, err := m.ValidateAccessToken(access)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.Username != "alice" {
		t.Errorf("username = %q, want 'alice'", claims.Username)
	}
	if claims.Role != RoleDeployer {
		t.Errorf("role = %q, want %q", claims.Role, RoleDeployer)
	}
	if claims.Issuer != "banyan" {
		t.Errorf("issuer = %q, want 'banyan'", claims.Issuer)
	}
}

func TestValidateAccessToken_Expired(t *testing.T) {
	key := make([]byte, 32)
	store := storage.NewMemoryStore()
	m := NewJWTManager(key, store)

	// Create a token that expired 1 second ago
	now := time.Now()
	claims := AccessClaims{
		Username: "alice",
		Role:     RoleViewer,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Second)),
			Issuer:    "banyan",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(key)

	_, err := m.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateAccessToken_TamperedSignature(t *testing.T) {
	m := newTestJWTManager()
	access, _, _ := m.CreateTokenPair("alice", RoleAdmin)

	// Tamper with a character mid-token. The final base64url char of an
	// HMAC-SHA256 signature carries only 4 significant bits, so flipping it
	// can be a no-op; a mid-token byte is always significant. The flip is
	// conditional so it is guaranteed to differ from the original.
	b := []byte(access)
	idx := len(b) / 2
	if b[idx] == 'A' {
		b[idx] = 'B'
	} else {
		b[idx] = 'A'
	}
	tampered := string(b)
	_, err := m.ValidateAccessToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestValidateAccessToken_WrongSigningKey(t *testing.T) {
	m1 := newTestJWTManager()
	access, _, _ := m1.CreateTokenPair("alice", RoleAdmin)

	// Validate with a different key
	differentKey := make([]byte, 32)
	for i := range differentKey {
		differentKey[i] = byte(i + 100)
	}
	m2 := NewJWTManager(differentKey, storage.NewMemoryStore())

	_, err := m2.ValidateAccessToken(access)
	if err == nil {
		t.Fatal("expected error for wrong signing key")
	}
}

func TestValidateAccessToken_MalformedToken(t *testing.T) {
	m := newTestJWTManager()
	_, err := m.ValidateAccessToken("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestValidateAccessToken_EmptyString(t *testing.T) {
	m := newTestJWTManager()
	_, err := m.ValidateAccessToken("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestValidateRefreshToken_HappyPath(t *testing.T) {
	m := newTestJWTManager()
	_, refresh, err := m.CreateTokenPair("alice", RoleDeployer)
	if err != nil {
		t.Fatalf("CreateTokenPair: %v", err)
	}

	username, err := m.ValidateRefreshToken(refresh)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if username != "alice" {
		t.Errorf("username = %q, want 'alice'", username)
	}
}

func TestValidateRefreshToken_Revoked(t *testing.T) {
	m := newTestJWTManager()
	_, refresh, _ := m.CreateTokenPair("alice", RoleAdmin)

	// Use it once (this revokes it)
	_, err := m.ValidateRefreshToken(refresh)
	if err != nil {
		t.Fatalf("first ValidateRefreshToken: %v", err)
	}

	// Second use should fail (token rotated/revoked)
	_, err = m.ValidateRefreshToken(refresh)
	if err == nil {
		t.Fatal("expected error for revoked refresh token")
	}
}

func TestValidateRefreshToken_InvalidToken(t *testing.T) {
	m := newTestJWTManager()
	_, err := m.ValidateRefreshToken("garbage")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
}

func TestRevokeRefreshToken_HappyPath(t *testing.T) {
	m := newTestJWTManager()
	_, refresh, _ := m.CreateTokenPair("alice", RoleAdmin)

	// Revoke
	err := m.RevokeRefreshToken(refresh)
	if err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}

	// Validate should fail
	_, err = m.ValidateRefreshToken(refresh)
	if err == nil {
		t.Fatal("expected error after revocation")
	}
}

func TestRevokeRefreshToken_AlreadyExpiredOrInvalid(t *testing.T) {
	m := newTestJWTManager()

	// Revoking garbage or expired tokens should not error
	if err := m.RevokeRefreshToken("garbage"); err != nil {
		t.Errorf("RevokeRefreshToken(garbage) should not error: %v", err)
	}
	if err := m.RevokeRefreshToken(""); err != nil {
		t.Errorf("RevokeRefreshToken('') should not error: %v", err)
	}
}

func TestCleanupExpiredTokens(t *testing.T) {
	key := make([]byte, 32)
	store := storage.NewMemoryStore()
	m := NewJWTManager(key, store)

	// Create a refresh token record that's already expired
	expired := RefreshTokenRecord{
		Username:  "alice",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	_ = store.Save(context.Background(), types.KeyAuthRefresh+"expired-jti", &expired)

	// Create a valid refresh token record
	valid := RefreshTokenRecord{
		Username:  "bob",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = store.Save(context.Background(), types.KeyAuthRefresh+"valid-jti", &valid)

	cleaned, err := m.CleanupExpiredTokens(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpiredTokens: %v", err)
	}
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1", cleaned)
	}

	// Valid token should still exist
	var check RefreshTokenRecord
	if err := store.Get(context.Background(), types.KeyAuthRefresh+"valid-jti", &check); err != nil {
		t.Error("valid refresh token should still exist after cleanup")
	}

	// Expired token should be gone
	if err := store.Get(context.Background(), types.KeyAuthRefresh+"expired-jti", &check); err == nil {
		t.Error("expired refresh token should have been cleaned up")
	}
}

func TestCleanupExpiredTokens_NoneExpired(t *testing.T) {
	m := newTestJWTManager()
	_, _, _ = m.CreateTokenPair("alice", RoleAdmin)
	_, _, _ = m.CreateTokenPair("bob", RoleViewer)

	cleaned, err := m.CleanupExpiredTokens(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpiredTokens: %v", err)
	}
	if cleaned != 0 {
		t.Errorf("cleaned = %d, want 0 (none expired)", cleaned)
	}
}

func TestTokenPairLifecycle(t *testing.T) {
	m := newTestJWTManager()

	// 1. Create tokens
	access, refresh, err := m.CreateTokenPair("alice", RoleDeployer)
	if err != nil {
		t.Fatalf("CreateTokenPair: %v", err)
	}

	// 2. Validate access token
	claims, err := m.ValidateAccessToken(access)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.Username != "alice" || claims.Role != RoleDeployer {
		t.Error("claims mismatch")
	}

	// 3. Validate + revoke refresh token (simulates refresh flow)
	username, err := m.ValidateRefreshToken(refresh)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if username != "alice" {
		t.Errorf("refresh username = %q, want 'alice'", username)
	}

	// 4. Create new pair with updated role (simulates role change picked up during refresh)
	access2, refresh2, err := m.CreateTokenPair("alice", RoleAdmin)
	if err != nil {
		t.Fatalf("CreateTokenPair (2): %v", err)
	}

	// 5. New access token has updated role
	claims2, err := m.ValidateAccessToken(access2)
	if err != nil {
		t.Fatalf("ValidateAccessToken (2): %v", err)
	}
	if claims2.Role != RoleAdmin {
		t.Errorf("new role = %q, want %q", claims2.Role, RoleAdmin)
	}

	// 6. Old refresh token is revoked
	_, err = m.ValidateRefreshToken(refresh)
	if err == nil {
		t.Fatal("old refresh token should be revoked")
	}

	// 7. New refresh token works
	_, err = m.ValidateRefreshToken(refresh2)
	if err != nil {
		t.Fatalf("new refresh token should be valid: %v", err)
	}
}

func TestValidateRefreshToken_ExpiredRecord(t *testing.T) {
	key := make([]byte, 32)
	store := storage.NewMemoryStore()
	m := NewJWTManager(key, store)

	// Create a token pair normally
	_, refresh, err := m.CreateTokenPair("alice", RoleAdmin)
	if err != nil {
		t.Fatalf("CreateTokenPair: %v", err)
	}

	// Manually expire the refresh token record in the store
	keys, _ := store.List(context.Background(), types.KeyAuthRefresh)
	for _, k := range keys {
		expired := RefreshTokenRecord{
			Username:  "alice",
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		_ = store.Save(context.Background(), k, &expired)
	}

	_, err = m.ValidateRefreshToken(refresh)
	if err == nil {
		t.Fatal("expected error for expired refresh token record")
	}
}

func TestCreateTokenPair_MultipleForSameUser(t *testing.T) {
	m := newTestJWTManager()

	// Create two pairs for the same user (e.g., two devices)
	_, refresh1, _ := m.CreateTokenPair("alice", RoleAdmin)
	_, refresh2, _ := m.CreateTokenPair("alice", RoleAdmin)

	if refresh1 == refresh2 {
		t.Error("each pair should have a unique refresh token")
	}

	// Both should be independently valid
	u1, err := m.ValidateRefreshToken(refresh1)
	if err != nil {
		t.Fatalf("refresh1 should be valid: %v", err)
	}
	if u1 != "alice" {
		t.Errorf("refresh1 username = %q", u1)
	}

	u2, err := m.ValidateRefreshToken(refresh2)
	if err != nil {
		t.Fatalf("refresh2 should be valid: %v", err)
	}
	if u2 != "alice" {
		t.Errorf("refresh2 username = %q", u2)
	}
}

func TestGenerateTokenID(t *testing.T) {
	id1, err := generateTokenID()
	if err != nil {
		t.Fatalf("generateTokenID: %v", err)
	}
	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("token ID length = %d, want 32", len(id1))
	}

	id2, _ := generateTokenID()
	if id1 == id2 {
		t.Error("two generated IDs should not be equal")
	}
}
