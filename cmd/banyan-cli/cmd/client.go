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

// NewAutoEngineClient creates a gRPC client that connects to the engine.
// Always uses the `engines` list from config. Each engine has its own WireGuard
// public key — the CLI derives tunnel IPs from keys and connects via WG tunnel.
// Supports multiple engines for HA failover — tries each with a health check.
func NewAutoEngineClient(engineAddr string) (*EngineClient, error) {
	cfg, _ := types.LoadConfig(configPath)

	// Build engine list from config
	engines := types.ResolveCLIEngines(&cfg.CLI)
	if len(engines) == 0 {
		return nil, fmt.Errorf("no engines configured. Run 'sudo banyan-cli init'")
	}

	// WG auth required unless engines are configured for direct connection (e.g., insecure mode)
	if cfg.CLI.WGPublicKey == "" && len(cfg.CLI.Engines) == 0 {
		return nil, fmt.Errorf("no authentication configured. Run 'sudo banyan-cli init'")
	}

	// Build gRPC endpoint list
	tunnelActive := controlTunnelExistsFn(types.ControlIfaceCLI)
	endpoints := types.BuildGRPCEndpoints(engines, tunnelActive)

	// Try each endpoint with a health check
	var lastErr error
	for _, ep := range endpoints {
		client, err := NewEngineClient(ep)
		if err != nil {
			lastErr = err
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = client.Health(ctx)
		cancel()
		if err != nil {
			client.Close()
			lastErr = err
			continue
		}
		return client, nil
	}
	return nil, fmt.Errorf("failed to connect to any engine: %w", lastErr)
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

// CreateSecret creates or updates a secret on the engine.
func (c *EngineClient) CreateSecret(ctx context.Context, name string, value []byte) error {
	_, err := c.client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{Name: name, Value: value})
	return err
}

// ListSecrets returns all secret metadata from the engine.
func (c *EngineClient) ListSecrets(ctx context.Context) ([]*banyanpb.SecretInfo, error) {
	resp, err := c.client.ListSecrets(ctx, &banyanpb.ListSecretsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Secrets, nil
}

// GetSecret returns secret metadata, optionally with the decrypted value.
func (c *EngineClient) GetSecret(ctx context.Context, name string, reveal bool) (*banyanpb.GetSecretResponse, error) {
	return c.client.GetSecret(ctx, &banyanpb.GetSecretRequest{Name: name, Reveal: reveal})
}

// DeleteSecret removes a secret from the engine.
func (c *EngineClient) DeleteSecret(ctx context.Context, name string) error {
	_, err := c.client.DeleteSecret(ctx, &banyanpb.DeleteSecretRequest{Name: name})
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
			Restart:     svc.Restart,
			Entrypoint:  svc.Entrypoint,
		}
		if len(svc.DependsOn) > 0 {
			ms.DependsOn = make(map[string]*banyanpb.DependsOnCondition, len(svc.DependsOn))
			for depName, dep := range svc.DependsOn {
				ms.DependsOn[depName] = &banyanpb.DependsOnCondition{Condition: dep.Condition}
			}
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
			if svc.Deploy.Autoscale != nil {
				md.Autoscale = &banyanpb.ManifestAutoscale{
					Min:       int32(svc.Deploy.Autoscale.Min),       //nolint:gosec // small int
					Max:       int32(svc.Deploy.Autoscale.Max),       //nolint:gosec // small int
					TargetCpu: int32(svc.Deploy.Autoscale.TargetCPU), //nolint:gosec // small int
					Cooldown:  svc.Deploy.Autoscale.Cooldown,
				}
			}
			if svc.Deploy.StopGracePeriod != "" {
				md.StopGracePeriod = svc.Deploy.StopGracePeriod
			}
			ms.Deploy = md
		}
		// Volume mounts
		for _, vol := range svc.Volumes {
			pbVol := &banyanpb.VolumeMount{
				Type:     vol.Type,
				Source:   vol.Source,
				Target:   vol.Target,
				ReadOnly: vol.ReadOnly,
			}
			if vol.Tmpfs != nil {
				pbVol.Tmpfs = &banyanpb.TmpfsOpt{Size: vol.Tmpfs.Size}
			}
			ms.Volumes = append(ms.Volumes, pbVol)
		}

		ms.Secrets = svc.Secrets

		services[name] = ms
	}

	// Top-level volume configs
	var pbVolumes map[string]*banyanpb.VolumeConfig
	if len(m.Volumes) > 0 {
		pbVolumes = make(map[string]*banyanpb.VolumeConfig, len(m.Volumes))
		for name, vc := range m.Volumes {
			pbVolumes[name] = &banyanpb.VolumeConfig{
				Driver:     vc.Driver,
				DriverOpts: vc.DriverOpts,
				External:   vc.External,
				Name:       vc.Name,
			}
		}
	}

	return &banyanpb.Manifest{
		Name:     m.Name,
		Version:  m.Version,
		Services: services,
		Volumes:  pbVolumes,
	}
}
