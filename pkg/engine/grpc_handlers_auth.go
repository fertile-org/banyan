package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/engine/auth"
	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	banyanpb "github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// Login authenticates a user with username+password and returns a JWT pair.
func (s *engineGRPCServer) Login(ctx context.Context, req *banyanpb.LoginRequest) (*banyanpb.LoginResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	// Rate limit login attempts
	peerIP, _ := banyanrpc.PeerIPFromContext(ctx)
	if s.loginLimiter != nil && !s.loginLimiter.allow(peerIP) {
		s.logLoginEvent(peerIP, req.Username, false, "rate limited")
		return nil, status.Errorf(codes.ResourceExhausted,
			"too many login attempts. Try again in 1 minute.")
	}

	if req.Username == "" || req.Password == "" {
		s.logLoginEvent(peerIP, req.Username, false, "empty credentials")
		return nil, status.Errorf(codes.InvalidArgument, "username and password are required")
	}

	user, err := s.authDeps.Users.Get(ctx, req.Username)
	if err != nil {
		// Same error for user-not-found and wrong-password (prevent enumeration)
		s.logLoginEvent(peerIP, req.Username, false, "invalid credentials")
		return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}

	if user.Disabled {
		s.logLoginEvent(peerIP, req.Username, false, "account disabled")
		return nil, status.Errorf(codes.PermissionDenied, "account disabled. Contact your administrator.")
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		s.logLoginEvent(peerIP, req.Username, false, "invalid credentials")
		return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}

	accessToken, refreshToken, err := s.authDeps.JWT.CreateTokenPair(user.Username, user.Role)
	if err != nil {
		s.logLoginEvent(peerIP, req.Username, false, "token creation failed")
		return nil, status.Errorf(codes.Internal, "failed to create session")
	}

	s.logLoginEvent(peerIP, req.Username, true, "")

	return &banyanpb.LoginResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		Username:         user.Username,
		Role:             user.Role,
		ExpiresInSeconds: int64(auth.AccessTokenExpiry.Seconds()),
	}, nil
}

// RefreshToken validates a refresh token and returns a new token pair.
func (s *engineGRPCServer) RefreshToken(ctx context.Context, req *banyanpb.RefreshTokenRequest) (*banyanpb.RefreshTokenResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	if req.RefreshToken == "" {
		return nil, status.Errorf(codes.InvalidArgument, "refresh token is required")
	}

	username, err := s.authDeps.JWT.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "session expired. Run: banyan login")
	}

	// Look up current role from store (pick up role changes)
	user, err := s.authDeps.Users.Get(ctx, username)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "user account not found")
	}
	if user.Disabled {
		return nil, status.Errorf(codes.PermissionDenied, "account disabled")
	}

	accessToken, refreshToken, err := s.authDeps.JWT.CreateTokenPair(user.Username, user.Role)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create session")
	}

	return &banyanpb.RefreshTokenResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresInSeconds: int64(auth.AccessTokenExpiry.Seconds()),
	}, nil
}

// CreateUser creates a new user (admin only, enforced by middleware).
func (s *engineGRPCServer) CreateUser(ctx context.Context, req *banyanpb.CreateUserRequest) (*banyanpb.CreateUserResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	if req.Username == "" || req.Password == "" || req.Role == "" {
		return nil, status.Errorf(codes.InvalidArgument, "username, password, and role are required")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process password")
	}

	user := &auth.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
		CreatedBy:    auth.UsernameFromContext(ctx),
	}

	if err := s.authDeps.Users.Create(ctx, user); err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "%v", err)
	}

	return &banyanpb.CreateUserResponse{}, nil
}

// ListUsers returns all users (admin only).
func (s *engineGRPCServer) ListUsers(ctx context.Context, _ *banyanpb.ListUsersRequest) (*banyanpb.ListUsersResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	users, err := s.authDeps.Users.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}

	var infos []*banyanpb.UserInfo
	for _, u := range users {
		infos = append(infos, &banyanpb.UserInfo{
			Username:  u.Username,
			Role:      u.Role,
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
			CreatedBy: u.CreatedBy,
			Disabled:  u.Disabled,
		})
	}

	return &banyanpb.ListUsersResponse{Users: infos}, nil
}

// DeleteUser removes a user (admin only).
func (s *engineGRPCServer) DeleteUser(ctx context.Context, req *banyanpb.DeleteUserRequest) (*banyanpb.DeleteUserResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	if req.Username == "" {
		return nil, status.Errorf(codes.InvalidArgument, "username is required")
	}

	if err := s.authDeps.Users.Delete(ctx, req.Username); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	}

	return &banyanpb.DeleteUserResponse{}, nil
}

// UpdateUserRole changes a user's role (admin only).
func (s *engineGRPCServer) UpdateUserRole(ctx context.Context, req *banyanpb.UpdateUserRoleRequest) (*banyanpb.UpdateUserRoleResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	if req.Username == "" || req.Role == "" {
		return nil, status.Errorf(codes.InvalidArgument, "username and role are required")
	}

	if err := s.authDeps.Users.Update(ctx, &auth.User{
		Username: req.Username,
		Role:     req.Role,
	}); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	}

	return &banyanpb.UpdateUserRoleResponse{}, nil
}

// ChangePassword changes the caller's own password (any authenticated user).
func (s *engineGRPCServer) ChangePassword(ctx context.Context, req *banyanpb.ChangePasswordRequest) (*banyanpb.ChangePasswordResponse, error) {
	if s.authDeps == nil {
		return nil, status.Errorf(codes.Unimplemented, "authentication not configured")
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		return nil, status.Errorf(codes.InvalidArgument, "current and new passwords are required")
	}

	username := auth.UsernameFromContext(ctx)
	if username == "" {
		return nil, status.Errorf(codes.Unauthenticated, "not authenticated")
	}

	user, err := s.authDeps.Users.Get(ctx, username)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}

	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		return nil, status.Errorf(codes.Unauthenticated, "current password is incorrect")
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process password")
	}

	if err := s.authDeps.Users.Update(ctx, &auth.User{
		Username:     username,
		PasswordHash: newHash,
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update password: %v", err)
	}

	return &banyanpb.ChangePasswordResponse{}, nil
}

func (s *engineGRPCServer) logLoginEvent(ip, username string, success bool, reason string) {
	if success {
		slog.Info("login success", "username", username, "ip", ip)
	} else {
		slog.Warn("login failed", "username", username, "ip", ip, "reason", reason)
	}
	if s.events != nil {
		msg := fmt.Sprintf("Login success for user %q from %s", username, ip)
		severity := "info"
		if !success {
			msg = fmt.Sprintf("Login failed for user %q from %s: %s", username, ip, reason)
			severity = "warning"
		}
		s.events.Add(Event{
			Timestamp: time.Now(),
			Type:      "auth.login",
			Message:   msg,
			Severity:  severity,
		})
	}
}
