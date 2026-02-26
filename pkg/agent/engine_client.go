package agent

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/fertile-org/banyan/pkg/metrics"
	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// EngineClient wraps the gRPC connection to the engine.
type EngineClient struct {
	conn   *grpc.ClientConn
	client banyanpb.EngineServiceClient
}

// NewEngineClient dials the engine gRPC server with public key credentials.
func NewEngineClient(engineAddr, publicKey string) (*EngineClient, error) {
	conn, err := grpc.NewClient(engineAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.PublicKeyCredentials{PublicKey: publicKey}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine at %s: %w", engineAddr, err)
	}

	return &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}, nil
}

// VPCConfig holds VPC networking configuration returned by the engine during registration.
type VPCConfig struct {
	VPCCIDR         string
	AllocatedSubnet string // /24 subnet allocated for this agent
	OverlayType     string // "wireguard" or "vxlan"
}

// VPCPeer represents a remote agent in the overlay network.
type VPCPeer struct {
	Subnet    string
	HostIP    string
	VTEPMAC   string // VXLAN
	PublicKey string // WireGuard
}

// ActiveContainer describes a container previously running on this agent.
type ActiveContainer struct {
	ContainerName string
	ContainerIP   string
	Ports         []string
	ServiceName   string
	DeploymentID  string
	TaskID        string
}

func (c *EngineClient) Register(ctx context.Context, name, apiAddr, sessionToken string, tags []string, wgPublicKey string) (string, *VPCConfig, []ActiveContainer, error) {
	resp, err := c.client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    name,
		ApiAddress:   apiAddr,
		SessionToken: sessionToken,
		Tags:         tags,
		WgPublicKey:  wgPublicKey,
	})
	if err != nil {
		return "", nil, nil, fmt.Errorf("register failed: %w", err)
	}

	var vpcConfig *VPCConfig
	if resp.AllocatedSubnet != "" && resp.VpcCidr != "" {
		vpcConfig = &VPCConfig{
			VPCCIDR:         resp.VpcCidr,
			AllocatedSubnet: resp.AllocatedSubnet,
			OverlayType:     resp.OverlayType,
		}
	}

	var activeContainers []ActiveContainer
	for _, ac := range resp.ActiveContainers {
		activeContainers = append(activeContainers, ActiveContainer{
			ContainerName: ac.ContainerName,
			ContainerIP:   ac.ContainerIp,
			Ports:         ac.Ports,
			ServiceName:   ac.ServiceName,
			DeploymentID:  ac.DeploymentId,
			TaskID:        ac.TaskId,
		})
	}

	return resp.RegistryUrl, vpcConfig, activeContainers, nil
}

func (c *EngineClient) Heartbeat(ctx context.Context, name, sessionToken string, tags []string, sysMetrics metrics.SystemMetrics) ([]VPCPeer, []ServiceBackend, error) {
	resp, err := c.client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    name,
		SessionToken: sessionToken,
		Tags:         tags,
		SystemMetrics: &banyanpb.SystemMetrics{
			CpuUsageRatio:    sysMetrics.CPUUsageRatio,
			MemoryUsedBytes:  sysMetrics.MemoryUsedBytes,
			MemoryTotalBytes: sysMetrics.MemoryTotalBytes,
			DiskUsedBytes:    sysMetrics.DiskUsedBytes,
			DiskTotalBytes:   sysMetrics.DiskTotalBytes,
			CpuCores:         sysMetrics.CPUCores,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("heartbeat failed: %w", err)
	}

	var peers []VPCPeer
	for _, p := range resp.VpcPeers {
		peers = append(peers, VPCPeer{
			Subnet:    p.Subnet,
			HostIP:    p.HostIp,
			VTEPMAC:   p.VtepMac,
			PublicKey: p.PublicKey,
		})
	}

	var backends []ServiceBackend
	for _, b := range resp.ServiceBackends {
		backends = append(backends, ServiceBackend{
			ContainerName: b.ContainerName,
			ContainerIP:   b.ContainerIp,
			Ports:         b.Ports,
			AgentName:     b.AgentName,
			ServiceName:   b.ServiceName,
		})
	}

	return peers, backends, nil
}

func (c *EngineClient) PollTasks(ctx context.Context, name string) ([]*banyanpb.TaskRecord, error) {
	resp, err := c.client.PollTasks(ctx, &banyanpb.PollTasksRequest{
		AgentName: name,
	})
	if err != nil {
		return nil, fmt.Errorf("poll tasks failed: %w", err)
	}
	return resp.Tasks, nil
}

func (c *EngineClient) ReportTaskResult(ctx context.Context, taskID, agentID, status, errMsg, containerName string, result *banyanpb.TaskResult) error {
	_, err := c.client.ReportTaskResult(ctx, &banyanpb.ReportTaskResultRequest{
		TaskId:        taskID,
		AgentId:       agentID,
		Status:        status,
		Error:         errMsg,
		ContainerName: containerName,
		Result:        result,
	})
	if err != nil {
		return fmt.Errorf("report task result failed: %w", err)
	}
	return nil
}

func (c *EngineClient) ReportContainerHealth(ctx context.Context, agentName string, containers []*banyanpb.ContainerStatus) error {
	_, err := c.client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName:  agentName,
		Containers: containers,
	})
	if err != nil {
		return fmt.Errorf("report container health failed: %w", err)
	}
	return nil
}

func (c *EngineClient) Health(ctx context.Context) error {
	_, err := c.client.Health(ctx, &banyanpb.HealthRequest{})
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

func (c *EngineClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
