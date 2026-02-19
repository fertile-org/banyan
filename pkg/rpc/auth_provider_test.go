package rpc

import (
	"context"
	"testing"

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
