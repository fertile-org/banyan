package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestPasswordAuthProvider(t *testing.T) {
	password := "test-password"
	provider := &PasswordAuthProvider{PasswordHash: HashPassword(password)}

	t.Run("valid password", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, password))
		if err := provider.Validate(ctx); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, "wrong"))
		err := provider.Validate(ctx)
		if err == nil {
			t.Fatal("expected error for wrong password")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("missing metadata", func(t *testing.T) {
		err := provider.Validate(context.Background())
		if err == nil {
			t.Fatal("expected error for missing metadata")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})
}

func TestAuthUnaryInterceptor(t *testing.T) {
	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	t.Run("with PasswordAuthProvider valid", func(t *testing.T) {
		password := "test-password"
		provider := &PasswordAuthProvider{PasswordHash: HashPassword(password)}
		interceptor := AuthUnaryInterceptor(provider)
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, password))
		resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if resp != "ok" {
			t.Errorf("expected 'ok', got %v", resp)
		}
	})

	t.Run("with PasswordAuthProvider invalid", func(t *testing.T) {
		provider := &PasswordAuthProvider{PasswordHash: HashPassword("correct")}
		interceptor := AuthUnaryInterceptor(provider)
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, "wrong"))
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("with NoAuthProvider always passes", func(t *testing.T) {
		provider := &NoAuthProvider{}
		interceptor := AuthUnaryInterceptor(provider)
		resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if resp != "ok" {
			t.Errorf("expected 'ok', got %v", resp)
		}
	})
}

func TestAuthStreamInterceptor(t *testing.T) {
	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	t.Run("with PasswordAuthProvider valid", func(t *testing.T) {
		password := "test-password"
		provider := &PasswordAuthProvider{PasswordHash: HashPassword(password)}
		interceptor := AuthStreamInterceptor(provider)
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, password))
		err := interceptor(nil, &providerMockStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("with PasswordAuthProvider invalid", func(t *testing.T) {
		provider := &PasswordAuthProvider{PasswordHash: HashPassword("correct")}
		interceptor := AuthStreamInterceptor(provider)
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, "wrong"))
		err := interceptor(nil, &providerMockStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("with NoAuthProvider always passes", func(t *testing.T) {
		provider := &NoAuthProvider{}
		interceptor := AuthStreamInterceptor(provider)
		err := interceptor(nil, &providerMockStream{ctx: context.Background()}, &grpc.StreamServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

// providerMockStream implements grpc.ServerStream for testing.
type providerMockStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *providerMockStream) Context() context.Context { return m.ctx }

func TestNoAuthProvider(t *testing.T) {
	provider := &NoAuthProvider{}

	t.Run("always succeeds", func(t *testing.T) {
		if err := provider.Validate(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("succeeds without metadata", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
		if err := provider.Validate(ctx); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}
