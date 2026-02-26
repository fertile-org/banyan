package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestSessionTokenCredentials(t *testing.T) {
	creds := &SessionTokenCredentials{Token: "tok-abc"}

	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md[MetadataKeySessionToken] != "tok-abc" {
		t.Errorf("expected token 'tok-abc', got %q", md[MetadataKeySessionToken])
	}

	if creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to return false")
	}
}

func TestPublicKeyCredentials(t *testing.T) {
	creds := &PublicKeyCredentials{PublicKey: "my-pubkey-base64"}

	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md[MetadataKeyPublicKey] != "my-pubkey-base64" {
		t.Errorf("expected pubkey 'my-pubkey-base64', got %q", md[MetadataKeyPublicKey])
	}

	if creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to return false")
	}
}

func TestPublicKeyValidator(t *testing.T) {
	validator := &PublicKeyValidator{AllowedKeys: map[string]string{
		"allowed-key": "worker-1",
	}}

	if err := validator.Validate("allowed-key"); err != nil {
		t.Fatalf("expected no error for allowed key, got %v", err)
	}

	if err := validator.Validate("unknown-key"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestValidateSessionToken(t *testing.T) {
	token := "session-token-abc123"

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr codes.Code
	}{
		{
			name:    "valid token",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeySessionToken, token)),
			wantErr: codes.OK,
		},
		{
			name:    "wrong token",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeySessionToken, "wrong")),
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "missing metadata",
			ctx:     context.Background(),
			wantErr: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionToken(tt.ctx, token)
			if tt.wantErr == codes.OK {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if got := status.Code(err); got != tt.wantErr {
				t.Errorf("expected code %v, got %v", tt.wantErr, got)
			}
		})
	}
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

func TestNewPublicKeyAuthInterceptor(t *testing.T) {
	validator := &PublicKeyValidator{AllowedKeys: map[string]string{
		"valid-pubkey": "worker-1",
	}}
	interceptor := NewPublicKeyAuthInterceptor(validator)

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	t.Run("valid public key passes", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPublicKey, "valid-pubkey"))
		info := &grpc.UnaryServerInfo{FullMethod: "/banyan.v1.EngineService/Register"}
		resp, err := interceptor(ctx, nil, info, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if resp != "ok" {
			t.Errorf("expected 'ok', got %v", resp)
		}
	})

	t.Run("invalid public key rejected", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPublicKey, "bad-key"))
		info := &grpc.UnaryServerInfo{FullMethod: "/banyan.v1.EngineService/Register"}
		_, err := interceptor(ctx, nil, info, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("missing public key rejected", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
		info := &grpc.UnaryServerInfo{FullMethod: "/banyan.v1.EngineService/Deploy"}
		_, err := interceptor(ctx, nil, info, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("Health RPC is exempt", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
		info := &grpc.UnaryServerInfo{FullMethod: "/banyan.v1.EngineService/Health"}
		resp, err := interceptor(ctx, nil, info, handler)
		if err != nil {
			t.Fatalf("expected no error for Health, got %v", err)
		}
		if resp != "ok" {
			t.Errorf("expected 'ok', got %v", resp)
		}
	})
}

func TestNewPublicKeyAuthStreamInterceptor(t *testing.T) {
	validator := &PublicKeyValidator{AllowedKeys: map[string]string{
		"valid-pubkey": "worker-1",
	}}
	interceptor := NewPublicKeyAuthStreamInterceptor(validator)

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	t.Run("valid key passes", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPublicKey, "valid-pubkey"))
		err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("invalid key rejected", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPublicKey, "bad-key"))
		err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})
}

func TestSessionTokenAuthInterceptor(t *testing.T) {
	token := "my-session-token"
	interceptor := SessionTokenAuthInterceptor(func() string { return token })

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	t.Run("valid token passes through", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeySessionToken, token))
		resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if resp != "ok" {
			t.Errorf("expected 'ok', got %v", resp)
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeySessionToken, "wrong"))
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})
}

func TestSessionTokenAuthStreamInterceptor(t *testing.T) {
	token := "my-session-token"
	interceptor := SessionTokenAuthStreamInterceptor(func() string { return token })

	handler := func(srv any, stream grpc.ServerStream) error {
		return nil
	}

	t.Run("valid token passes through", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeySessionToken, token))
		err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeySessionToken, "wrong"))
		err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
		if err == nil {
			t.Fatal("expected error")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})
}
