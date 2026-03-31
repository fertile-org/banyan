// grpc_server.go contains the engine gRPC server setup, interceptors, and shared helpers.
// Handler implementations are split into separate files:
//   - grpc_handlers_agent.go     — agent RPCs (Register, Heartbeat, PollTasks, etc.)
//   - grpc_handlers_cli.go       — CLI RPCs (Deploy, Down, GetStatus, Scale, etc.)
//   - grpc_handlers_dashboard.go — dashboard RPC (GetDashboardData)
//   - grpc_handlers_secrets.go   — secret management RPCs (CreateSecret, etc.)
package engine

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/metrics"
	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// rateLimiter provides per-IP request rate limiting.
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time // IP → timestamps of recent requests
	limit    int                    // max requests per window
	window   time.Duration          // sliding window duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// allow returns true if the request from the given IP is within rate limits.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Trim old entries
	timestamps := rl.requests[ip]
	start := 0
	for start < len(timestamps) && timestamps[start].Before(cutoff) {
		start++
	}
	timestamps = timestamps[start:]

	if len(timestamps) >= rl.limit {
		rl.requests[ip] = timestamps
		return false
	}

	rl.requests[ip] = append(timestamps, now)
	return true
}

// engineGRPCServer implements the EngineService gRPC server.
type engineGRPCServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store           storage.StateStore
	registryURL     string
	whitelistedKeys map[string]string                // publicKey → agentName
	tunnelIPToAgent map[string]string                // tunnelIP → agentName (reverse map for identity)
	allocator       overlay.SubnetAllocatorInterface // VPC subnet allocator (nil if VPC disabled)
	peerTracker     overlay.PeerTrackerInterface     // VPC peer tracker (nil if VPC disabled)
	vpcCIDR         string                           // VPC network CIDR (e.g., "10.0.0.0/16")
	scheduleCh      chan<- struct{}                   // triggers immediate scheduling (nil-safe)
	metricsRegistry *metrics.EngineMetricsRegistry
	events          EventLog
	secrets         *SecretsManager // nil if secrets.key not present
	limiter         *rateLimiter
	startedAt       time.Time
	log             *logging.Logger
}

// grpcServerOptions configures the engine gRPC server.
type grpcServerOptions struct {
	Store           storage.StateStore
	Port            string
	BindAddr        string // address to bind to (default: control tunnel IP or 127.0.0.1)
	RegistryURL     string
	Allocator       overlay.SubnetAllocatorInterface
	PeerTracker     overlay.PeerTrackerInterface
	VPCCIDR         string
	WhitelistedKeys map[string]string // publicKey → agentName
	AllowInsecure   bool              // allow running without authentication
	ScheduleCh      chan<- struct{}    // triggers immediate scheduling
	MetricsRegistry *metrics.EngineMetricsRegistry
	Events          EventLog
	Secrets         *SecretsManager // nil if secrets.key not present
	StartedAt       time.Time
}

// triggerSchedule signals the engine orchestration loop to process deployments immediately.
func (s *engineGRPCServer) triggerSchedule() {
	if s.scheduleCh == nil {
		return
	}
	select {
	case s.scheduleCh <- struct{}{}:
	default:
	}
}

// logger returns the gRPC server's logger, initializing it if nil (for test convenience).
func (s *engineGRPCServer) logger() *logging.Logger {
	if s.log == nil {
		s.log = logging.New("engine")
	}
	return s.log
}

// startEngineGRPC starts the gRPC server for agent communication.
func startEngineGRPC(ctx context.Context, opts *grpcServerOptions) (*engineGRPCServer, error) {
	bindAddr := opts.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	listenAddr := bindAddr + ":" + opts.Port
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on gRPC %s: %w", listenAddr, err)
	}

	// Build reverse map: tunnel IP → agent name (for caller identity from WireGuard)
	tunnelIPMap := make(map[string]string, len(opts.WhitelistedKeys))
	for pubKey, agentName := range opts.WhitelistedKeys {
		tunnelIP := types.TunnelIPFromPublicKey(pubKey)
		tunnelIPMap[tunnelIP.String()] = agentName
	}

	engineSrv := &engineGRPCServer{
		store:           opts.Store,
		registryURL:     opts.RegistryURL,
		whitelistedKeys: opts.WhitelistedKeys,
		tunnelIPToAgent: tunnelIPMap,
		allocator:       opts.Allocator,
		peerTracker:     opts.PeerTracker,
		vpcCIDR:         opts.VPCCIDR,
		scheduleCh:      opts.ScheduleCh,
		metricsRegistry: opts.MetricsRegistry,
		events:          opts.Events,
		secrets:         opts.Secrets,
		limiter:         newRateLimiter(100, time.Minute), // 100 requests/min per IP
		startedAt:       opts.StartedAt,
		log:             logging.New("engine"),
	}

	// WireGuard provides authentication at the network layer.
	// No application-layer auth interceptors needed — only peers with
	// whitelisted WireGuard keys can reach the tunnel IP.
	// Add audit logging, rate limiting, and panic recovery interceptors.
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			engineSrv.panicRecoveryInterceptor(),
			engineSrv.rateLimitInterceptor(),
			engineSrv.auditLogInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			engineSrv.panicRecoveryStreamInterceptor(),
			engineSrv.rateLimitStreamInterceptor(),
			engineSrv.auditLogStreamInterceptor(),
		),
	)
	if opts.AllowInsecure {
		engineSrv.logger().Warn("gRPC server running WITHOUT authentication (--allow-insecure)")
	}

	banyanpb.RegisterEngineServiceServer(srv, engineSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			engineSrv.logger().Error("gRPC server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()

	return engineSrv, nil
}

// agentNameFromContext resolves the agent name from the gRPC peer's tunnel IP.
func (s *engineGRPCServer) agentNameFromContext(ctx context.Context) string {
	ip, err := banyanrpc.PeerIPFromContext(ctx)
	if err != nil {
		return ""
	}
	return s.tunnelIPToAgent[ip]
}

// panicRecoveryInterceptor returns a unary interceptor that recovers from panics in handlers.
func (s *engineGRPCServer) panicRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				s.logger().Error("Panic in gRPC handler",
					"method", info.FullMethod, "panic", r)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// panicRecoveryStreamInterceptor returns a stream interceptor that recovers from panics.
func (s *engineGRPCServer) panicRecoveryStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				s.logger().Error("Panic in stream gRPC handler",
					"method", info.FullMethod, "panic", r)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// rateLimitInterceptor returns a unary interceptor that enforces per-IP rate limiting.
func (s *engineGRPCServer) rateLimitInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		peerIP, _ := banyanrpc.PeerIPFromContext(ctx)
		if peerIP != "" && !s.limiter.allow(peerIP) {
			s.logger().Warn("Rate limited", "peer_ip", peerIP, "method", info.FullMethod)
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded — try again later")
		}
		return handler(ctx, req)
	}
}

// rateLimitStreamInterceptor returns a stream interceptor that enforces per-IP rate limiting.
func (s *engineGRPCServer) rateLimitStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		peerIP, _ := banyanrpc.PeerIPFromContext(ss.Context())
		if peerIP != "" && !s.limiter.allow(peerIP) {
			s.logger().Warn("Rate limited (stream)", "peer_ip", peerIP, "method", info.FullMethod)
			return status.Errorf(codes.ResourceExhausted, "rate limit exceeded — try again later")
		}
		return handler(srv, ss)
	}
}

// isControlTunnelIP returns true if the IP is on the WireGuard control tunnel network.
// Any IP on this network is an authenticated WG peer (the tunnel enforces this).
func isControlTunnelIP(ip string) bool {
	_, cidr, _ := net.ParseCIDR(types.ControlTunnelCIDR)
	return cidr != nil && cidr.Contains(net.ParseIP(ip))
}

// auditLogInterceptor returns a unary interceptor that logs all RPC calls with peer identity.
func (s *engineGRPCServer) auditLogInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		peerIP, _ := banyanrpc.PeerIPFromContext(ctx)
		agent := s.tunnelIPToAgent[peerIP]

		// Warn about unknown peers — but not for IPs on the control tunnel
		// (same-host agent/CLI traffic may arrive from the engine's own tunnel IP)
		if len(s.tunnelIPToAgent) > 0 && agent == "" && peerIP != "" && !isControlTunnelIP(peerIP) {
			s.logger().Warn("RPC from unknown peer",
				"method", info.FullMethod, "peer_ip", peerIP)
		}

		resp, err := handler(ctx, req)
		if err != nil {
			s.logger().Debug("RPC failed",
				"method", info.FullMethod, "peer_ip", peerIP, "agent", agent, "error", err)
		}
		return resp, err
	}
}

// auditLogStreamInterceptor returns a stream interceptor that logs stream RPC calls with peer identity.
func (s *engineGRPCServer) auditLogStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		peerIP, _ := banyanrpc.PeerIPFromContext(ss.Context())
		agent := s.tunnelIPToAgent[peerIP]

		if len(s.tunnelIPToAgent) > 0 && agent == "" && peerIP != "" && !isControlTunnelIP(peerIP) {
			s.logger().Warn("Stream RPC from unknown peer",
				"method", info.FullMethod, "peer_ip", peerIP)
		}

		err := handler(srv, ss)
		if err != nil {
			s.logger().Debug("Stream RPC failed",
				"method", info.FullMethod, "peer_ip", peerIP, "agent", agent, "error", err)
		}
		return err
	}
}

// emitEvent adds an event to the buffer and increments the Prometheus counter.
func (s *engineGRPCServer) emitEvent(eventType, message, severity string) {
	if s.events != nil {
		s.events.Add(Event{
			Timestamp: time.Now(),
			Type:      eventType,
			Message:   message,
			Severity:  severity,
		})
	}
	if s.metricsRegistry != nil {
		s.metricsRegistry.IncrementEvent(eventType)
	}
}
