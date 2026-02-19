package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// mockLogProvider returns bytes from a buffer.
type mockLogProvider struct {
	data []byte
	err  error
}

func (m *mockLogProvider) StreamLogs(ctx context.Context, containerName string, opts types.LogOptions) (io.ReadCloser, error) {
	if m.err != nil {
		return nil, m.err
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

// nonstopLogReader produces data forever regardless of context.
// It does NOT respect context cancellation — the only way to stop it is via Close().
type nonstopLogReader struct {
	closed chan struct{}
}

func newNonstopLogReader() *nonstopLogReader {
	return &nonstopLogReader{closed: make(chan struct{})}
}

func (r *nonstopLogReader) Read(p []byte) (int, error) {
	select {
	case <-r.closed:
		return 0, io.EOF
	default:
		// Produce data without delay
		data := []byte("log line\n")
		n := copy(p, data)
		return n, nil
	}
}

func (r *nonstopLogReader) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

// nonstopLogProvider returns a reader that produces data indefinitely.
type nonstopLogProvider struct {
	reader *nonstopLogReader
}

func (p *nonstopLogProvider) StreamLogs(ctx context.Context, containerName string, opts types.LogOptions) (io.ReadCloser, error) {
	return p.reader, nil
}

func setupAgentServer(t *testing.T, logProvider types.LogProvider, sessionToken string) (banyanpb.AgentServiceClient, func()) {
	t.Helper()

	lis := bufconn.Listen(testBufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.SessionTokenAuthInterceptor(func() string { return sessionToken })),
		grpc.StreamInterceptor(banyanrpc.SessionTokenAuthStreamInterceptor(func() string { return sessionToken })),
	)

	agentSrv := &agentGRPCServer{logProvider: logProvider}
	banyanpb.RegisterAgentServiceServer(srv, agentSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.SessionTokenCredentials{Token: sessionToken}),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := banyanpb.NewAgentServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}

	return client, cleanup
}

func TestStreamLogs(t *testing.T) {
	t.Run("empty container name error", func(t *testing.T) {
		client, cleanup := setupAgentServer(t, &mockLogProvider{data: []byte("logs")}, "token-123")
		defer cleanup()

		stream, err := client.StreamLogs(context.Background(), &banyanpb.StreamLogsRequest{
			ContainerName: "",
		})
		if err != nil {
			t.Fatalf("StreamLogs call failed: %v", err)
		}

		// Should get an error on Recv
		_, recvErr := stream.Recv()
		if recvErr == nil {
			t.Fatal("expected error for empty container name")
		}
		if status.Code(recvErr) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", status.Code(recvErr))
		}
	})

	t.Run("streams log data", func(t *testing.T) {
		logData := []byte("hello from container logs\n")
		client, cleanup := setupAgentServer(t, &mockLogProvider{data: logData}, "token-123")
		defer cleanup()

		stream, err := client.StreamLogs(context.Background(), &banyanpb.StreamLogsRequest{
			ContainerName: "myapp-web-0",
		})
		if err != nil {
			t.Fatalf("StreamLogs call failed: %v", err)
		}

		var received []byte
		for {
			resp, recvErr := stream.Recv()
			if recvErr != nil {
				break
			}
			received = append(received, resp.Data...)
		}

		if !bytes.Equal(received, logData) {
			t.Errorf("expected %q, got %q", logData, received)
		}
	})

	t.Run("provider error returns Internal", func(t *testing.T) {
		provider := &mockLogProvider{err: fmt.Errorf("container not found")}
		client, cleanup := setupAgentServer(t, provider, "token-123")
		defer cleanup()

		stream, err := client.StreamLogs(context.Background(), &banyanpb.StreamLogsRequest{
			ContainerName: "nonexistent-container",
		})
		if err != nil {
			t.Fatalf("StreamLogs call failed: %v", err)
		}

		_, recvErr := stream.Recv()
		if recvErr == nil {
			t.Fatal("expected error when provider fails")
		}
		if status.Code(recvErr) != codes.Internal {
			t.Errorf("expected Internal, got %v", status.Code(recvErr))
		}
	})

	t.Run("client disconnect stops stream", func(t *testing.T) {
		// Use a nonstop log provider that produces data regardless of context.
		// Cancel the client context mid-stream to trigger the stream.Send error path.
		reader := newNonstopLogReader()
		provider := &nonstopLogProvider{reader: reader}
		client, cleanup := setupAgentServer(t, provider, "token-123")
		defer cleanup()
		defer reader.Close()

		ctx, cancel := context.WithCancel(context.Background())

		stream, err := client.StreamLogs(ctx, &banyanpb.StreamLogsRequest{
			ContainerName: "myapp-web-0",
		})
		if err != nil {
			t.Fatalf("StreamLogs call failed: %v", err)
		}

		// Receive a few messages to confirm the stream is working
		for i := 0; i < 3; i++ {
			_, recvErr := stream.Recv()
			if recvErr != nil {
				t.Fatalf("unexpected error on recv %d: %v", i, recvErr)
			}
		}

		// Cancel the client context to disconnect mid-stream.
		// The server should detect the broken stream when Send fails.
		cancel()

		// Allow the server goroutine to detect the disconnect
		time.Sleep(100 * time.Millisecond)
	})
}

func TestStartAgentGRPC(t *testing.T) {
	t.Run("starts and stops without panic", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		provider := &mockLogProvider{data: []byte("test logs")}

		// Use port "0" to let the OS assign a free port
		startAgentGRPC(ctx, provider, "0", "test-session-token")

		// Give the server a moment to start
		time.Sleep(50 * time.Millisecond)

		// Cancel context to trigger graceful stop
		cancel()

		// Give the server a moment to stop
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("handles listen failure on invalid port", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		provider := &mockLogProvider{data: []byte("test logs")}

		// Use an invalid port number to trigger a listen failure
		startAgentGRPC(ctx, provider, "99999", "test-session-token")
	})

	t.Run("starts server and connects client", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		provider := &mockLogProvider{data: []byte("server log data\n")}

		// Find a free port by binding temporarily
		lis, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("failed to find free port: %v", err)
		}
		port := lis.Addr().(*net.TCPAddr).Port
		lis.Close()

		portStr := fmt.Sprintf("%d", port)
		startAgentGRPC(ctx, provider, portStr, "test-token-123")

		// Give the server a moment to start
		time.Sleep(100 * time.Millisecond)

		// Connect a client to the running server
		addr := fmt.Sprintf("localhost:%s", portStr)
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.SessionTokenCredentials{Token: "test-token-123"}),
		)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		client := banyanpb.NewAgentServiceClient(conn)
		stream, err := client.StreamLogs(ctx, &banyanpb.StreamLogsRequest{
			ContainerName: "test-container",
		})
		if err != nil {
			t.Fatalf("StreamLogs call failed: %v", err)
		}

		var received []byte
		for {
			resp, recvErr := stream.Recv()
			if recvErr != nil {
				break
			}
			received = append(received, resp.Data...)
		}

		if !bytes.Equal(received, []byte("server log data\n")) {
			t.Errorf("expected 'server log data\\n', got %q", received)
		}
	})
}
