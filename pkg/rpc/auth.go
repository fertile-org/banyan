package rpc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// MetadataKeyPassword is the gRPC metadata key for password auth (ExchangeToken only).
	MetadataKeyPassword = "x-banyan-password" //nolint:gosec // not a credential, just a metadata key name
	// MetadataKeyAuthToken is the gRPC metadata key for token auth (all other RPCs).
	MetadataKeyAuthToken = "x-banyan-auth-token" //nolint:gosec // not a credential, just a metadata key name
	// MetadataKeySessionToken is the gRPC metadata key for engine→agent session auth.
	MetadataKeySessionToken = "x-banyan-session-token" //nolint:gosec // not a credential, just a metadata key name
)

// PasswordCredentials implements credentials.PerRPCCredentials for ExchangeToken calls.
// It attaches the cluster password to every RPC.
type PasswordCredentials struct {
	Password string
}

func (c *PasswordCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		MetadataKeyPassword: c.Password,
	}, nil
}

func (c *PasswordCredentials) RequireTransportSecurity() bool {
	return false
}

// Verify interface compliance.
var _ credentials.PerRPCCredentials = (*PasswordCredentials)(nil)

// TokenCredentials implements credentials.PerRPCCredentials for token-based auth.
// It attaches the auth token to every RPC.
type TokenCredentials struct {
	Token string
}

func (c *TokenCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		MetadataKeyAuthToken: c.Token,
	}, nil
}

func (c *TokenCredentials) RequireTransportSecurity() bool {
	return false
}

var _ credentials.PerRPCCredentials = (*TokenCredentials)(nil)

// SessionTokenCredentials implements credentials.PerRPCCredentials for engine→agent calls.
// It attaches the session token to every RPC.
type SessionTokenCredentials struct {
	Token string
}

func (c *SessionTokenCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		MetadataKeySessionToken: c.Token,
	}, nil
}

func (c *SessionTokenCredentials) RequireTransportSecurity() bool {
	return false
}

var _ credentials.PerRPCCredentials = (*SessionTokenCredentials)(nil)

// HashPassword returns the SHA-256 hex digest of the given password.
func HashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h)
}

// TokenValidator validates auth tokens against the store.
type TokenValidator interface {
	ValidateToken(ctx context.Context, tokenHash string) error
}

// NewAuthInterceptor returns a unary server interceptor with dual-mode auth:
//   - ExchangeToken RPC: validates password via bcrypt against passwordHash
//   - All other RPCs: validates token via TokenValidator
func NewAuthInterceptor(passwordHash string, validator TokenValidator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isExchangeTokenMethod(info.FullMethod) {
			if err := validatePasswordBcrypt(ctx, passwordHash); err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}
		if err := validateAuthToken(ctx, validator); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// NewAuthStreamInterceptor returns a stream server interceptor with dual-mode auth.
// Stream RPCs always use token auth (ExchangeToken is unary-only).
func NewAuthStreamInterceptor(validator TokenValidator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := validateAuthToken(ss.Context(), validator); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func isExchangeTokenMethod(fullMethod string) bool {
	return strings.HasSuffix(fullMethod, "/ExchangeToken")
}

func validatePasswordBcrypt(ctx context.Context, passwordHash string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get(MetadataKeyPassword)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(values[0])); err != nil {
		return status.Error(codes.Unauthenticated, "invalid password")
	}
	return nil
}

func validateAuthToken(ctx context.Context, validator TokenValidator) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get(MetadataKeyAuthToken)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing auth token")
	}
	tokenHash := HashPassword(values[0])
	return validator.ValidateToken(ctx, tokenHash)
}

// --- Legacy interceptors kept for agent gRPC server (session token auth) ---

// SessionTokenAuthInterceptor returns a unary server interceptor that validates
// the session token in gRPC metadata against the locally held token.
func SessionTokenAuthInterceptor(getToken func() string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := validateSessionToken(ctx, getToken()); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// SessionTokenAuthStreamInterceptor returns a stream server interceptor that validates
// the session token in gRPC metadata.
func SessionTokenAuthStreamInterceptor(getToken func() string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := validateSessionToken(ss.Context(), getToken()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func validateSessionToken(ctx context.Context, expected string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get(MetadataKeySessionToken)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing session token")
	}
	if subtle.ConstantTimeCompare([]byte(values[0]), []byte(expected)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid session token")
	}
	return nil
}
