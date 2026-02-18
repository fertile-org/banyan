package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestPasswordCredentials(t *testing.T) {
	creds := &PasswordCredentials{Password: "secret123"}

	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md[MetadataKeyPassword] != "secret123" {
		t.Errorf("expected password 'secret123', got %q", md[MetadataKeyPassword])
	}

	if creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to return false")
	}
}

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

func TestValidatePassword(t *testing.T) {
	password := "mypassword"
	passwordHash := HashPassword(password)

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr codes.Code
	}{
		{
			name:    "valid password",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, password)),
			wantErr: codes.OK,
		},
		{
			name:    "wrong password",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKeyPassword, "wrong")),
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "missing metadata",
			ctx:     context.Background(),
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "empty metadata",
			ctx:     metadata.NewIncomingContext(context.Background(), metadata.MD{}),
			wantErr: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.ctx, passwordHash)
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

func TestHashPassword(t *testing.T) {
	hash1 := HashPassword("test")
	hash2 := HashPassword("test")
	if hash1 != hash2 {
		t.Error("same password should produce same hash")
	}

	hash3 := HashPassword("other")
	if hash1 == hash3 {
		t.Error("different passwords should produce different hashes")
	}
}
