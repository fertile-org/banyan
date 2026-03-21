package agent

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/logging"
	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

// agentGRPCServer implements the AgentService gRPC server for log streaming.
type agentGRPCServer struct {
	banyanpb.UnimplementedAgentServiceServer
	logProvider types.LogProvider
}

// engineIPAuthInterceptor returns a unary interceptor that verifies
// the caller is the engine by checking the peer IP matches the control tunnel engine IP.
func engineIPAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := verifyEngineIP(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// engineIPAuthStreamInterceptor returns a stream interceptor that verifies
// the caller is the engine by checking the peer IP.
func engineIPAuthStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := verifyEngineIP(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func verifyEngineIP(ctx context.Context) error {
	ip, err := banyanrpc.PeerIPFromContext(ctx)
	if err != nil {
		return err
	}
	// Verify the caller is on the control tunnel network (10.200.0.0/16).
	// Any engine with a valid WireGuard key will have a tunnel IP in this range.
	_, tunnelCIDR, _ := net.ParseCIDR(types.ControlTunnelCIDR)
	if tunnelCIDR != nil && !tunnelCIDR.Contains(net.ParseIP(ip)) {
		return status.Errorf(codes.PermissionDenied, "only the engine can connect to the agent gRPC server")
	}
	return nil
}

// startAgentGRPC starts the agent's gRPC server for log streaming.
// It binds to the agent's tunnel IP and verifies callers are the engine.
func startAgentGRPC(ctx context.Context, logProvider types.LogProvider, bindAddr, port string) {
	log := logging.New("agent.grpc")
	listenAddr := bindAddr + ":" + port
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Error("Failed to listen", "addr", listenAddr, "error", err)
		return
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(engineIPAuthInterceptor()),
		grpc.StreamInterceptor(engineIPAuthStreamInterceptor()),
	)

	agentSrv := &agentGRPCServer{logProvider: logProvider}
	banyanpb.RegisterAgentServiceServer(srv, agentSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("gRPC server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()

	log.Info("gRPC server listening", "addr", listenAddr)
}

func (s *agentGRPCServer) StreamLogs(req *banyanpb.StreamLogsRequest, stream banyanpb.AgentService_StreamLogsServer) error {
	if req.ContainerName == "" {
		return status.Error(codes.InvalidArgument, "container_name is required")
	}

	opts := types.LogOptions{
		Follow: req.Follow,
		Tail:   int(req.Tail),
	}

	reader, err := s.logProvider.StreamLogs(stream.Context(), req.ContainerName, opts)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to stream logs: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if err := stream.Send(&banyanpb.StreamLogsResponse{Data: buf[:n]}); err != nil {
				return err
			}
		}
		if readErr != nil {
			return nil // EOF or error, stream ends
		}
	}
}
