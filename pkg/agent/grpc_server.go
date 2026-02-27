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

// startAgentGRPC starts the agent's gRPC server for log streaming.
func startAgentGRPC(ctx context.Context, logProvider types.LogProvider, port, sessionToken string) {
	log := logging.New("agent.grpc")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Error("Failed to listen", "port", port, "error", err)
		return
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.SessionTokenAuthInterceptor(func() string { return sessionToken })),
		grpc.StreamInterceptor(banyanrpc.SessionTokenAuthStreamInterceptor(func() string { return sessionToken })),
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

	log.Info("gRPC server listening", "port", port)
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
