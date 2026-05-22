package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/storage"
)

func newTestAuthDeps(t *testing.T) (*AuthDeps, *EtcdUserStore, *JWTManager) {
	t.Helper()
	store := storage.NewMemoryStore()
	jwtKey := make([]byte, 32)
	for i := range jwtKey {
		jwtKey[i] = byte(i)
	}
	jm := NewJWTManager(jwtKey, store)
	us := NewEtcdUserStore(store)
	az := NewRoleAuthorizer()

	return &AuthDeps{JWT: jm, Users: us, Authorizer: az}, us, jm
}

func createUserForAuth(t *testing.T, us *EtcdUserStore, username, role string) {
	t.Helper()
	hash, _ := HashPassword("password")
	_ = us.Create(context.Background(), &User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		CreatedBy:    "test",
	})
}

func TestAuthenticateAndAuthorize_BypassMethod(t *testing.T) {
	deps, _, _ := newTestAuthDeps(t)
	ctx := context.Background()

	// Agent RPCs bypass auth
	ctx2, err := authenticateAndAuthorize(ctx, "/banyan.v1.EngineService/Register", "", deps)
	if err != nil {
		t.Fatalf("bypass method should not error: %v", err)
	}
	if ctx2 == nil {
		t.Fatal("context should not be nil")
	}
}

func TestAuthenticateAndAuthorize_NoToken(t *testing.T) {
	deps, _, _ := newTestAuthDeps(t)
	_, err := authenticateAndAuthorize(context.Background(), "/banyan.v1.EngineService/Deploy", "", deps)
	if err == nil {
		t.Fatal("expected error when no token provided")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestAuthenticateAndAuthorize_InvalidToken(t *testing.T) {
	deps, _, _ := newTestAuthDeps(t)
	_, err := authenticateAndAuthorize(context.Background(), "/banyan.v1.EngineService/Deploy", "bad-token", deps)
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestAuthenticateAndAuthorize_ValidToken_Allowed(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "alice", RoleDeployer)

	access, _, _ := jm.CreateTokenPair("alice", RoleDeployer)
	ctx, err := authenticateAndAuthorize(context.Background(), "/banyan.v1.EngineService/Deploy", access, deps)
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if UsernameFromContext(ctx) != "alice" {
		t.Errorf("expected alice in context, got %q", UsernameFromContext(ctx))
	}
	if RoleFromContext(ctx) != RoleDeployer {
		t.Errorf("expected deployer in context, got %q", RoleFromContext(ctx))
	}
}

func TestAuthenticateAndAuthorize_ValidToken_Denied(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "viewer1", RoleViewer)

	access, _, _ := jm.CreateTokenPair("viewer1", RoleViewer)
	_, err := authenticateAndAuthorize(context.Background(), "/banyan.v1.EngineService/Deploy", access, deps)
	if err == nil {
		t.Fatal("viewer should be denied Deploy")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
	// Check actionable error message
	if msg := st.Message(); msg == "" {
		t.Error("error message should not be empty")
	}
}

func TestAuthenticateAndAuthorize_DisabledUser(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "admin1", RoleAdmin)
	createUserForAuth(t, us, "disabled1", RoleDeployer)

	// Disable the user
	_ = us.Update(context.Background(), &User{Username: "disabled1", Disabled: true})

	access, _, _ := jm.CreateTokenPair("disabled1", RoleDeployer)
	_, err := authenticateAndAuthorize(context.Background(), "/banyan.v1.EngineService/Deploy", access, deps)
	if err == nil {
		t.Fatal("disabled user should be denied")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestAuthenticateAndAuthorize_RoleChangeInstantEffect(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "admin1", RoleAdmin)
	createUserForAuth(t, us, "bob", RoleDeployer)

	// Create token while bob is deployer
	access, _, _ := jm.CreateTokenPair("bob", RoleDeployer)

	// Change bob's role to viewer
	_ = us.Update(context.Background(), &User{Username: "bob", Role: RoleViewer})

	// Token still says deployer, but store says viewer — should be denied
	_, err := authenticateAndAuthorize(context.Background(), "/banyan.v1.EngineService/Deploy", access, deps)
	if err == nil {
		t.Fatal("bob's role was changed to viewer, Deploy should be denied")
	}
}

func TestAuthenticateAndAuthorize_UnknownMethod(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "alice", RoleAdmin)

	access, _, _ := jm.CreateTokenPair("alice", RoleAdmin)
	_, err := authenticateAndAuthorize(context.Background(), "/unknown/Method", access, deps)
	if err == nil {
		t.Fatal("unknown method should be denied")
	}
}

func TestExtractBearerFromGRPC(t *testing.T) {
	tests := []struct {
		name  string
		md    metadata.MD
		want  string
	}{
		{"valid bearer", metadata.Pairs("authorization", "Bearer mytoken"), "mytoken"},
		{"lowercase bearer", metadata.Pairs("authorization", "bearer mytoken"), "mytoken"},
		{"no auth header", metadata.MD{}, ""},
		{"no bearer prefix", metadata.Pairs("authorization", "Basic abc"), ""},
		{"empty value", metadata.Pairs("authorization", ""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), tt.md)
			got := extractBearerFromGRPC(ctx)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractBearerFromGRPC_NoMetadata(t *testing.T) {
	got := extractBearerFromGRPC(context.Background())
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractBearerFromHTTP_Header(t *testing.T) {
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	got := extractBearerFromHTTP(req)
	if got != "mytoken" {
		t.Errorf("got %q, want 'mytoken'", got)
	}
}

func TestExtractBearerFromHTTP_Cookie(t *testing.T) {
	req := httptest.NewRequest("POST", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "banyan_token", Value: "cookietoken"})
	got := extractBearerFromHTTP(req)
	if got != "cookietoken" {
		t.Errorf("got %q, want 'cookietoken'", got)
	}
}

func TestExtractBearerFromHTTP_HeaderOverCookie(t *testing.T) {
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Authorization", "Bearer headertoken")
	req.AddCookie(&http.Cookie{Name: "banyan_token", Value: "cookietoken"})
	got := extractBearerFromHTTP(req)
	if got != "headertoken" {
		t.Errorf("header should take precedence, got %q", got)
	}
}

func TestExtractBearerFromHTTP_Nothing(t *testing.T) {
	req := httptest.NewRequest("POST", "/test", nil)
	got := extractBearerFromHTTP(req)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestConnectAuthMiddleware_Bypass(t *testing.T) {
	deps, _, _ := newTestAuthDeps(t)
	middleware := ConnectAuthMiddleware(deps)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/banyan.v1.EngineService/Health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called for bypass method")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestConnectAuthMiddleware_Unauthenticated(t *testing.T) {
	deps, _, _ := newTestAuthDeps(t)
	middleware := ConnectAuthMiddleware(deps)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/banyan.v1.EngineService/Deploy", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestConnectAuthMiddleware_Forbidden(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "viewer1", RoleViewer)
	access, _, _ := jm.CreateTokenPair("viewer1", RoleViewer)

	middleware := ConnectAuthMiddleware(deps)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/banyan.v1.EngineService/Deploy", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestConnectAuthMiddleware_Authorized(t *testing.T) {
	deps, us, jm := newTestAuthDeps(t)
	createUserForAuth(t, us, "deployer1", RoleDeployer)
	access, _, _ := jm.CreateTokenPair("deployer1", RoleDeployer)

	called := false
	middleware := ConnectAuthMiddleware(deps)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if UsernameFromContext(r.Context()) != "deployer1" {
			t.Error("username not in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/banyan.v1.EngineService/Deploy", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called for authorized request")
	}
}
