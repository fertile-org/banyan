package engine

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// streamAgentLogs connects to an agent's gRPC server and streams logs as an io.ReadCloser.
// No application-layer auth is needed — the agent verifies the caller is the engine
// by checking the peer IP matches the engine's control tunnel IP.
func streamAgentLogs(ctx context.Context, agentAddr, containerName string, follow bool, tail int32) (io.ReadCloser, error) {
	// Use passthrough scheme for direct IP:port addresses. grpc.NewClient defaults
	// to the "dns" resolver which can silently fail on bare IP addresses.
	conn, err := grpc.NewClient("passthrough:///"+agentAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent at %s: %w", agentAddr, err)
	}

	client := banyanpb.NewAgentServiceClient(conn)
	stream, err := client.StreamLogs(ctx, &banyanpb.StreamLogsRequest{
		ContainerName: containerName,
		Follow:        follow,
		Tail:          tail,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to start log stream: %w", err)
	}

	return &grpcLogReader{stream: stream, conn: conn}, nil
}

// grpcLogReader wraps a gRPC stream as an io.ReadCloser.
type grpcLogReader struct {
	stream banyanpb.AgentService_StreamLogsClient
	conn   *grpc.ClientConn
	buf    []byte
}

func (r *grpcLogReader) Read(p []byte) (int, error) {
	// Drain buffered data first
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	resp, err := r.stream.Recv()
	if err != nil {
		return 0, err
	}

	n := copy(p, resp.Data)
	if n < len(resp.Data) {
		r.buf = resp.Data[n:]
	}
	return n, nil
}

func (r *grpcLogReader) Close() error {
	r.conn.Close()
	return nil
}
