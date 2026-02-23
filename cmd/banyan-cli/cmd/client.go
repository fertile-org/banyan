package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

// EngineClient is a gRPC client for the Engine API.
type EngineClient struct {
	conn   *grpc.ClientConn
	client banyanpb.EngineServiceClient
}

// NewEngineClient creates a new engine gRPC client with token-based auth.
func NewEngineClient(engineAddr, token string) (*EngineClient, error) {
	conn, err := grpc.NewClient(engineAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: token}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine at %s: %w", engineAddr, err)
	}

	return &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *EngineClient) Close() {
	c.conn.Close()
}

// Deploy creates a new deployment via the engine gRPC API.
// If services is non-empty, only those services will be redeployed.
func (c *EngineClient) Deploy(ctx context.Context, manifest types.BanyanManifest, services []string) (*banyanpb.DeployRPCResponse, error) {
	return c.client.Deploy(ctx, &banyanpb.DeployRPCRequest{
		Manifest: manifestToProto(manifest),
		Services: services,
	})
}

// Down stops services in a deployment via the engine gRPC API.
func (c *EngineClient) Down(ctx context.Context, name string, services []string) (*banyanpb.DownRPCResponse, error) {
	return c.client.Down(ctx, &banyanpb.DownRPCRequest{
		Name:     name,
		Services: services,
	})
}

// Status retrieves cluster status from the engine gRPC API.
func (c *EngineClient) Status(ctx context.Context) (*banyanpb.GetStatusResponse, error) {
	return c.client.GetStatus(ctx, &banyanpb.GetStatusRequest{})
}

// Info retrieves engine info (registry URL).
func (c *EngineClient) Info(ctx context.Context) (*banyanpb.GetInfoResponse, error) {
	return c.client.GetInfo(ctx, &banyanpb.GetInfoRequest{})
}

// StreamLogs streams logs for a container from the engine gRPC API.
func (c *EngineClient) StreamLogs(ctx context.Context, container string, follow bool, tail int) (io.ReadCloser, error) {
	stream, err := c.client.GetLogs(ctx, &banyanpb.GetLogsRequest{
		ContainerName: container,
		Follow:        follow,
		Tail:          int32(tail), //nolint:gosec // tail count is always small
	})
	if err != nil {
		return nil, fmt.Errorf("failed to stream logs: %w", err)
	}
	return &grpcLogStreamReader{stream: stream}, nil
}

// Health checks if the engine is healthy.
func (c *EngineClient) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.client.Health(ctx, &banyanpb.HealthRequest{})
	return err
}

// grpcLogStreamReader wraps a gRPC server-streaming response as io.ReadCloser.
type grpcLogStreamReader struct {
	stream grpc.ServerStreamingClient[banyanpb.GetLogsResponse]
	buf    []byte
}

func (r *grpcLogStreamReader) Read(p []byte) (int, error) {
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

func (r *grpcLogStreamReader) Close() error {
	return nil
}

// manifestToProto converts types.BanyanManifest to proto Manifest.
func manifestToProto(m types.BanyanManifest) *banyanpb.Manifest {
	services := make(map[string]*banyanpb.ManifestService, len(m.Services))
	for name, svc := range m.Services { //nolint:gocritic // map iteration
		ms := &banyanpb.ManifestService{
			Image:       svc.Image,
			Ports:       svc.Ports,
			Environment: svc.Environment,
			Command:     svc.Command,
			DependsOn:   svc.DependsOn,
		}
		if svc.Build != nil {
			ms.Build = &banyanpb.ManifestBuild{
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
			}
		}
		if svc.Deploy != nil {
			ms.Deploy = &banyanpb.ManifestDeploy{
				Replicas: int32(svc.Deploy.Replicas), //nolint:gosec // replica count is always small
			}
		}
		services[name] = ms
	}
	return &banyanpb.Manifest{
		Name:     m.Name,
		Version:  m.Version,
		Services: services,
	}
}
