package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

const (
	// AccessTokenExpiry is how long an access token is valid.
	AccessTokenExpiry = 1 * time.Hour

	// RefreshTokenExpiry is how long a refresh token is valid.
	RefreshTokenExpiry = 7 * 24 * time.Hour
)

// AccessClaims are the JWT claims in an access token.
type AccessClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// RefreshClaims are the JWT claims in a refresh token.
type RefreshClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// RefreshTokenRecord is stored in etcd for revocation tracking.
type RefreshTokenRecord struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

// JWTManager handles JWT creation, validation, and refresh token lifecycle.
type JWTManager struct {
	signingKey []byte
	store      storage.StateStore
}

// NewJWTManager creates a JWTManager with the given HMAC-SHA256 signing key
// and a StateStore for refresh token revocation tracking.
func NewJWTManager(signingKey []byte, store storage.StateStore) *JWTManager {
	return &JWTManager{
		signingKey: signingKey,
		store:      store,
	}
}

// CreateTokenPair generates a new access token and refresh token for the given user.
func (m *JWTManager) CreateTokenPair(username, role string) (accessToken, refreshToken string, err error) {
	now := time.Now()

	// Access token: short-lived, contains role for authorization
	accessClaims := AccessClaims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenExpiry)),
			Issuer:    "banyan",
		},
	}
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err = at.SignedString(m.signingKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %w", err)
	}

	// Refresh token: long-lived, has a unique ID for revocation
	jti, err := generateTokenID()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate token ID: %w", err)
	}
	refreshClaims := RefreshClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTokenExpiry)),
			Issuer:    "banyan",
		},
	}
	rt := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err = rt.SignedString(m.signingKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	// Store the refresh token JTI in etcd for revocation tracking
	record := RefreshTokenRecord{
		Username:  username,
		ExpiresAt: now.Add(RefreshTokenExpiry),
	}
	if err := m.store.Save(context.Background(), types.KeyAuthRefresh+jti, &record); err != nil {
		return "", "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

// ValidateAccessToken parses and validates an access token.
// Returns the claims if valid, or an error describing why validation failed.
func (m *JWTManager) ValidateAccessToken(tokenString string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid access token: %w", err)
	}

	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid access token claims")
	}
	return claims, nil
}

// ValidateRefreshToken validates a refresh token and revokes it (token rotation).
// Returns the username from the token. The caller should look up the user's current
// role from the UserStore before calling CreateTokenPair for the new pair.
func (m *JWTManager) ValidateRefreshToken(refreshTokenString string) (username string, err error) {
	token, err := jwt.ParseWithClaims(refreshTokenString, &RefreshClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return "", fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.signingKey, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid refresh token: %w", err)
	}

	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid refresh token claims")
	}

	// Check the JTI exists in etcd (not revoked)
	jti := claims.ID
	var record RefreshTokenRecord
	if err := m.store.Get(context.Background(), types.KeyAuthRefresh+jti, &record); err != nil {
		return "", fmt.Errorf("refresh token has been revoked")
	}

	// Check expiry from the stored record
	if time.Now().After(record.ExpiresAt) {
		_ = m.store.Delete(context.Background(), types.KeyAuthRefresh+jti)
		return "", fmt.Errorf("refresh token has expired")
	}

	// Revoke the old refresh token (rotation: old token can't be reused)
	_ = m.store.Delete(context.Background(), types.KeyAuthRefresh+jti)

	return claims.Username, nil
}

// RevokeRefreshToken deletes a refresh token from the store (used by logout).
func (m *JWTManager) RevokeRefreshToken(refreshTokenString string) error {
	token, err := jwt.ParseWithClaims(refreshTokenString, &RefreshClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.signingKey, nil
	})
	if err != nil {
		// Token might already be expired — that's fine, just clean up
		return nil
	}

	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return nil
	}

	_ = m.store.Delete(context.Background(), types.KeyAuthRefresh+claims.ID)
	return nil
}

// CleanupExpiredTokens removes refresh token records that have expired.
// Call this periodically (e.g., every hour) from the engine's main loop.
func (m *JWTManager) CleanupExpiredTokens(ctx context.Context) (int, error) {
	keys, err := m.store.List(ctx, types.KeyAuthRefresh)
	if err != nil {
		return 0, fmt.Errorf("failed to list refresh tokens: %w", err)
	}

	cleaned := 0
	now := time.Now()
	for _, k := range keys {
		var record RefreshTokenRecord
		if err := m.store.Get(ctx, k, &record); err != nil {
			continue
		}
		if now.After(record.ExpiresAt) {
			_ = m.store.Delete(ctx, k)
			cleaned++
		}
	}
	return cleaned, nil
}

// generateTokenID creates a random hex-encoded token ID.
func generateTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
