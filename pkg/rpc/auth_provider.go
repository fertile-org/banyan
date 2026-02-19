package rpc

import (
	"context"

	"google.golang.org/grpc"
)

// AuthProvider validates authentication for incoming gRPC requests.
type AuthProvider interface {
	// Validate checks the authentication credentials in the context.
	// Returns nil if valid, or a gRPC status error if not.
	Validate(ctx context.Context) error
}

// PasswordAuthProvider validates requests using password-based authentication.
type PasswordAuthProvider struct {
	PasswordHash string
}

func (p *PasswordAuthProvider) Validate(ctx context.Context) error {
	return validatePassword(ctx, p.PasswordHash)
}

// NoAuthProvider allows all requests without authentication.
type NoAuthProvider struct{}

func (p *NoAuthProvider) Validate(ctx context.Context) error {
	return nil
}

// AuthUnaryInterceptor returns a unary server interceptor that delegates to the given AuthProvider.
func AuthUnaryInterceptor(provider AuthProvider) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := provider.Validate(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// AuthStreamInterceptor returns a stream server interceptor that delegates to the given AuthProvider.
func AuthStreamInterceptor(provider AuthProvider) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := provider.Validate(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}
