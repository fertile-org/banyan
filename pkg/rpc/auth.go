package rpc

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// MetadataKeySessionToken is the gRPC metadata key for engine→agent session auth.
	MetadataKeySessionToken = "x-banyan-session-token" //nolint:gosec // not a credential, just a metadata key name
	// MetadataKeyPublicKey is the gRPC metadata key for public key auth.
	MetadataKeyPublicKey = "x-banyan-public-key" //nolint:gosec // not a credential, just a metadata key name
)

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

// PublicKeyCredentials implements credentials.PerRPCCredentials for public key auth.
// It attaches the agent's public key to every RPC.
type PublicKeyCredentials struct {
	PublicKey string
}

func (c *PublicKeyCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		MetadataKeyPublicKey: c.PublicKey,
	}, nil
}

func (c *PublicKeyCredentials) RequireTransportSecurity() bool {
	return false
}

var _ credentials.PerRPCCredentials = (*PublicKeyCredentials)(nil)

// PublicKeyValidator validates public keys against a whitelist.
type PublicKeyValidator struct {
	AllowedKeys map[string]string // publicKey → agent name
}

// Validate checks if a public key is in the whitelist.
func (v *PublicKeyValidator) Validate(publicKey string) error {
	if _, ok := v.AllowedKeys[publicKey]; !ok {
		return status.Error(codes.Unauthenticated, "public key not whitelisted")
	}
	return nil
}

// NewPublicKeyAuthInterceptor returns a unary server interceptor that validates
// public keys from gRPC metadata against the whitelist.
// Health RPCs are exempt (used for connectivity checks).
func NewPublicKeyAuthInterceptor(validator *PublicKeyValidator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isHealthMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		if err := validatePublicKey(ctx, validator); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// NewPublicKeyAuthStreamInterceptor returns a stream server interceptor that validates
// public keys from gRPC metadata against the whitelist.
func NewPublicKeyAuthStreamInterceptor(validator *PublicKeyValidator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := validatePublicKey(ss.Context(), validator); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func isHealthMethod(fullMethod string) bool {
	return strings.HasSuffix(fullMethod, "/Health")
}

func validatePublicKey(ctx context.Context, validator *PublicKeyValidator) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get(MetadataKeyPublicKey)
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing public key")
	}
	return validator.Validate(values[0])
}

// --- Session token auth interceptors (used by agent gRPC server) ---

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
