package agent

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// EngineClient wraps the gRPC connection to the engine.
// Supports multiple endpoints for HA failover.
type EngineClient struct {
	endpoints  []string
	currentIdx int
	conn       *grpc.ClientConn
	client     banyanpb.EngineServiceClient
	mu         sync.Mutex
}

// NewEngineClient dials the engine gRPC server.
// Authentication is handled by WireGuard at the network layer — only peers
// with whitelisted keys can reach the engine's control tunnel IP.
func NewEngineClient(engineAddr string) (*EngineClient, error) {
	return NewEngineClientMulti([]string{engineAddr})
}

// NewEngineClientMulti creates an EngineClient with multiple endpoints for HA failover.
// Connects to the first endpoint initially.
func NewEngineClientMulti(endpoints []string) (*EngineClient, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no engine endpoints provided")
	}
	ec := &EngineClient{endpoints: endpoints}
	if err := ec.connectTo(0); err != nil {
		return nil, err
	}
	return ec, nil
}

// connectTo connects to the endpoint at the given index.
func (ec *EngineClient) connectTo(idx int) error {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.conn != nil {
		ec.conn.Close()
	}
	conn, err := grpc.NewClient(ec.endpoints[idx],
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to engine at %s: %w", ec.endpoints[idx], err)
	}
	ec.conn = conn
	ec.client = banyanpb.NewEngineServiceClient(conn)
	ec.currentIdx = idx
	return nil
}

// Failover connects to the next endpoint in the list.
func (ec *EngineClient) Failover() error {
	next := (ec.currentIdx + 1) % len(ec.endpoints)
	return ec.connectTo(next)
}

// CurrentEndpoint returns the currently connected endpoint address.
func (ec *EngineClient) CurrentEndpoint() string {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.endpoints[ec.currentIdx]
}

// VPCConfig holds VPC networking configuration returned by the engine during registration.
type VPCConfig struct {
	VPCCIDR         string
	AllocatedSubnet string // /24 subnet allocated for this agent
}

// VPCPeer represents a remote agent in the overlay network.
type VPCPeer struct {
	Subnet    string
	HostIP    string
	PublicKey string // WireGuard public key
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

// RegisterRequest bundles the parameters for agent registration.
type RegisterRequest struct {
	Name        string
	APIAddr     string
	Tags        []string
	WGPublicKey string
	HostIP      string
}

func (ec *EngineClient) Register(ctx context.Context, req RegisterRequest) (string, *VPCConfig, []ActiveContainer, error) {
	resp, err := ec.client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:   req.Name,
		ApiAddress:  req.APIAddr,
		Tags:        req.Tags,
		WgPublicKey: req.WGPublicKey,
		HostIp:      req.HostIP,
		Arch:        runtime.GOARCH,
	})
	if err != nil {
		return "", nil, nil, fmt.Errorf("register failed: %w", err)
	}

	var vpcConfig *VPCConfig
	if resp.AllocatedSubnet != "" && resp.VpcCidr != "" {
		vpcConfig = &VPCConfig{
			VPCCIDR:         resp.VpcCidr,
			AllocatedSubnet: resp.AllocatedSubnet,
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

func (ec *EngineClient) Heartbeat(ctx context.Context, name string, tags []string, sysMetrics metrics.SystemMetrics) ([]VPCPeer, []ServiceBackend, error) {
	resp, err := ec.client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName: name,
		Tags:      tags,
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
			PublicKey: p.PublicKey,
		})
	}

	var backends []ServiceBackend
	for _, b := range resp.ServiceBackends {
		backends = append(backends, ServiceBackend{
			ContainerName:  b.ContainerName,
			ContainerIP:    b.ContainerIp,
			Ports:          b.Ports,
			AgentName:      b.AgentName,
			ServiceName:    b.ServiceName,
			DeploymentName: b.DeploymentName,
		})
	}

	return peers, backends, nil
}

func (ec *EngineClient) PollTasks(ctx context.Context, name string) ([]*banyanpb.TaskRecord, error) {
	resp, err := ec.client.PollTasks(ctx, &banyanpb.PollTasksRequest{
		AgentName: name,
	})
	if err != nil {
		return nil, fmt.Errorf("poll tasks failed: %w", err)
	}
	return resp.Tasks, nil
}

func (ec *EngineClient) ReportTaskResult(ctx context.Context, taskID, agentID, status, errMsg, containerName string, result *banyanpb.TaskResult) error {
	_, err := ec.client.ReportTaskResult(ctx, &banyanpb.ReportTaskResultRequest{
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

func (ec *EngineClient) ReportContainerHealth(ctx context.Context, agentName string, containers []*banyanpb.ContainerStatus) error {
	_, err := ec.client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName:  agentName,
		Containers: containers,
	})
	if err != nil {
		return fmt.Errorf("report container health failed: %w", err)
	}
	return nil
}

func (ec *EngineClient) Health(ctx context.Context) error {
	_, err := ec.client.Health(ctx, &banyanpb.HealthRequest{})
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

func (ec *EngineClient) Close() {
	if ec.conn != nil {
		ec.conn.Close()
	}
}
