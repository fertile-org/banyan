package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthDeps holds the dependencies needed by the auth middleware.
type AuthDeps struct {
	JWT        *JWTManager
	Users      UserStore
	Authorizer Authorizer
}

// authenticateAndAuthorize is the shared auth logic called by both
// gRPC interceptors and Connect-go HTTP middleware.
//
//	Auth flow:
//	  1. Bypass check (agent RPCs, Login/Refresh, Health)
//	  2. Extract JWT from bearer token
//	  3. Validate JWT signature + expiry
//	  4. Check user in store (instant role change / disable)
//	  5. Authorize via Authorizer interface
//	  6. Return context with identity injected
func authenticateAndAuthorize(ctx context.Context, fullMethod, bearerToken string, deps *AuthDeps) (context.Context, error) {
	// 1. Bypass methods (agent RPCs, auth RPCs, health)
	if IsBypassMethod(fullMethod) {
		return ctx, nil
	}

	// 2. Extract JWT
	if bearerToken == "" {
		return ctx, status.Errorf(codes.Unauthenticated,
			"authentication required. Run: banyan login")
	}

	// 3. Validate JWT
	claims, err := deps.JWT.ValidateAccessToken(bearerToken)
	if err != nil {
		return ctx, status.Errorf(codes.Unauthenticated,
			"invalid or expired token. Run: banyan login")
	}

	// 4. Check user in store for instant role change / disable
	user, err := deps.Users.Get(ctx, claims.Username)
	if err != nil {
		return ctx, status.Errorf(codes.Unauthenticated,
			"user account not found")
	}
	if user.Disabled {
		return ctx, status.Errorf(codes.PermissionDenied,
			"account disabled. Contact your administrator.")
	}

	// Use the role from the store (not from token) for instant effect
	currentRole := user.Role

	// 5. Authorize
	perm, ok := PermissionForMethod(fullMethod)
	if !ok {
		// Method not in permission map and not in bypass list = deny by default
		return ctx, status.Errorf(codes.PermissionDenied,
			"access denied for method %s", fullMethod)
	}

	if err := deps.Authorizer.Authorize(ctx, currentRole, perm.Resource, perm.Action); err != nil {
		var pde *PermissionDeniedError
		if errors.As(err, &pde) {
			return ctx, status.Errorf(codes.PermissionDenied,
				"permission denied. You need the '%s' role. "+
					"Ask your admin to run: banyan user set-role %s %s",
				pde.RequiredRole, claims.Username, pde.RequiredRole)
		}
		return ctx, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	// 6. Inject identity into context
	ctx = WithIdentity(ctx, claims.Username, currentRole)
	return ctx, nil
}

// extractBearerFromGRPC extracts a Bearer token from gRPC metadata.
func extractBearerFromGRPC(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return ""
	}
	// "Bearer <token>"
	parts := strings.SplitN(values[0], " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

// extractBearerFromHTTP extracts a Bearer token from HTTP request,
// checking the Authorization header first, then falling back to cookie.
func extractBearerFromHTTP(r *http.Request) string {
	// Try Authorization header
	auth := r.Header.Get("Authorization")
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
	}
	// Fall back to cookie
	cookie, err := r.Cookie("banyan_token")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// UnaryAuthInterceptor returns a gRPC unary server interceptor that
// enforces authentication and authorization.
func UnaryAuthInterceptor(deps *AuthDeps) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		token := extractBearerFromGRPC(ctx)
		ctx, err := authenticateAndAuthorize(ctx, info.FullMethod, token, deps)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor returns a gRPC stream server interceptor that
// enforces authentication and authorization at stream creation time.
func StreamAuthInterceptor(deps *AuthDeps) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		token := extractBearerFromGRPC(ss.Context())
		ctx, err := authenticateAndAuthorize(ss.Context(), info.FullMethod, token, deps)
		if err != nil {
			return err
		}
		return handler(srv, &authServerStream{ServerStream: ss, ctx: ctx})
	}
}

// authServerStream wraps a grpc.ServerStream to inject an authenticated context.
type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authServerStream) Context() context.Context {
	return s.ctx
}

// ConnectAuthMiddleware returns an HTTP middleware that enforces authentication
// and authorization for Connect-go (dashboard) requests.
func ConnectAuthMiddleware(deps *AuthDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Map the HTTP path to a gRPC-style method name
			// Connect-go paths look like: /banyan.v1.EngineService/MethodName
			fullMethod := r.URL.Path

			token := extractBearerFromHTTP(r)
			ctx, err := authenticateAndAuthorize(r.Context(), fullMethod, token, deps)
			if err != nil {
				st, ok := status.FromError(err)
				if !ok {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				switch st.Code() {
				case codes.Unauthenticated:
					http.Error(w, st.Message(), http.StatusUnauthorized)
				case codes.PermissionDenied:
					http.Error(w, st.Message(), http.StatusForbidden)
				default:
					http.Error(w, st.Message(), http.StatusInternalServerError)
				}
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// LoginRateLimiter wraps the rate limiter for login attempts.
// Returns a function that checks if the IP is allowed to attempt login.
// The rate limiter itself is created externally (reusing the existing rateLimiter type).
type LoginRateLimiter interface {
	Allow(ip string) bool
}

// FormatPermissionDeniedMessage creates an actionable error message for the CLI.
func FormatPermissionDeniedMessage(err error, username string) string {
	var pde *PermissionDeniedError
	if errors.As(err, &pde) {
		return fmt.Sprintf(
			"Permission denied. You need the '%s' role.\n"+
				"Ask your admin to run: banyan user set-role %s %s",
			pde.RequiredRole, username, pde.RequiredRole,
		)
	}
	return "Permission denied."
}
