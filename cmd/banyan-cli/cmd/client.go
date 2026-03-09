package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// EngineClient is a gRPC client for the Engine API.
type EngineClient struct {
	conn   *grpc.ClientConn
	client banyanpb.EngineServiceClient
}

// NewEngineClient creates a new engine gRPC client.
// Authentication is handled by WireGuard at the network layer.
func NewEngineClient(engineAddr string) (*EngineClient, error) {
	conn, err := grpc.NewClient(engineAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine at %s: %w", engineAddr, err)
	}

	return &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}, nil
}

// controlTunnelExistsFn is the function used to check if the WireGuard control tunnel
// kernel interface exists. Extracted as a package-level variable for test mocking.
var controlTunnelExistsFn = overlay.ControlTunnelExists

// controlTunnelEngineIP is the engine IP used when connecting through the control tunnel.
// Extracted as a package-level variable for test mocking.
var controlTunnelEngineIP = types.ControlTunnelEngineIP

// NewAutoEngineClient creates a gRPC client that connects through the WireGuard control tunnel.
// Requires both a WireGuard keypair and the engine's WG public key to be configured.
func NewAutoEngineClient(engineAddr string) (*EngineClient, error) {
	cfg, _ := types.LoadConfig(configPath)

	if cfg.CLI.WGPublicKey == "" {
		return nil, fmt.Errorf("no authentication configured. Run 'sudo banyan-cli init'")
	}

	if cfg.CLI.EngineWGPublicKey == "" {
		return nil, fmt.Errorf("engine WireGuard public key not configured. Run 'sudo banyan-cli init' and provide the engine's public key")
	}

	if !controlTunnelExistsFn(types.ControlIfaceCLI) {
		return nil, fmt.Errorf("WireGuard control tunnel is not active. Run 'sudo banyan-cli login' to re-establish it")
	}

	port := "50051"
	if cfg.CLI.EnginePort != "" {
		port = cfg.CLI.EnginePort
	}
	tunnelAddr := controlTunnelEngineIP + ":" + port
	return NewEngineClient(tunnelAddr)
}

// Close closes the gRPC connection.
func (c *EngineClient) Close() {
	c.conn.Close()
}

// Deploy creates a new deployment via the engine gRPC API.
// If services is non-empty, only those services will be redeployed.
func (c *EngineClient) Deploy(ctx context.Context, manifest types.BanyanManifest, services, tags []string) (*banyanpb.DeployRPCResponse, error) {
	return c.client.Deploy(ctx, &banyanpb.DeployRPCRequest{
		Manifest: manifestToProto(manifest),
		Services: services,
		Tags:     tags,
	})
}

// Down stops services in a deployment via the engine gRPC API.
func (c *EngineClient) Down(ctx context.Context, name string, services, tags []string) (*banyanpb.DownRPCResponse, error) {
	return c.client.Down(ctx, &banyanpb.DownRPCRequest{
		Name:     name,
		Services: services,
		Tags:     tags,
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

// DashboardData retrieves the full cluster snapshot for the dashboard.
func (c *EngineClient) DashboardData(ctx context.Context) (*banyanpb.GetDashboardDataResponse, error) {
	return c.client.GetDashboardData(ctx, &banyanpb.GetDashboardDataRequest{})
}

// GRPCClient returns the underlying gRPC client for direct use by the dashboard.
func (c *EngineClient) GRPCClient() banyanpb.EngineServiceClient {
	return c.client
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
			Restart:     svc.Restart,
			Entrypoint:  svc.Entrypoint,
		}
		if svc.Healthcheck != nil {
			ms.Healthcheck = &banyanpb.ManifestHealthcheck{
				Test:        svc.Healthcheck.Test,
				Interval:    svc.Healthcheck.Interval,
				Timeout:     svc.Healthcheck.Timeout,
				Retries:     int32(svc.Healthcheck.Retries), //nolint:gosec // retries is always small
				StartPeriod: svc.Healthcheck.StartPeriod,
				Disable:     svc.Healthcheck.Disable,
			}
		}
		if svc.Build != nil {
			ms.Build = &banyanpb.ManifestBuild{
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
			}
		}
		if svc.Deploy != nil {
			md := &banyanpb.ManifestDeploy{
				Replicas: int32(svc.Deploy.Replicas), //nolint:gosec // replica count is always small
			}
			if svc.Deploy.Placement != nil && svc.Deploy.Placement.Node != "" {
				md.Placement = &banyanpb.ManifestPlacement{
					Node: svc.Deploy.Placement.Node,
				}
			}
			if svc.Deploy.Resources != nil {
				mr := &banyanpb.ManifestResources{}
				if svc.Deploy.Resources.Limits != nil {
					mr.Limits = &banyanpb.ResourceSpec{
						Memory: svc.Deploy.Resources.Limits.Memory,
						Cpus:   svc.Deploy.Resources.Limits.CPUs,
					}
				}
				if svc.Deploy.Resources.Reservations != nil {
					mr.Reservations = &banyanpb.ResourceSpec{
						Memory: svc.Deploy.Resources.Reservations.Memory,
						Cpus:   svc.Deploy.Resources.Reservations.CPUs,
					}
				}
				md.Resources = mr
			}
			ms.Deploy = md
		}
		services[name] = ms
	}
	return &banyanpb.Manifest{
		Name:     m.Name,
		Version:  m.Version,
		Services: services,
	}
}
