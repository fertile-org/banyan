package agent

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// EngineClient wraps the gRPC connection to the engine.
type EngineClient struct {
	conn   *grpc.ClientConn
	client banyanpb.EngineServiceClient
}

// NewEngineClient dials the engine gRPC server with token credentials.
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

// NewEngineClientWithPassword dials the engine gRPC server with password credentials.
// Used during init to call ExchangeToken.
func NewEngineClientWithPassword(engineAddr, password string) (*EngineClient, error) {
	conn, err := grpc.NewClient(engineAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: password}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine at %s: %w", engineAddr, err)
	}

	return &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}, nil
}

// ExchangeToken calls the ExchangeToken RPC to get an auth token.
func (c *EngineClient) ExchangeToken(ctx context.Context, name, role string) (string, error) {
	resp, err := c.client.ExchangeToken(ctx, &banyanpb.ExchangeTokenRequest{
		Name: name,
		Role: role,
	})
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}
	return resp.Token, nil
}

func (c *EngineClient) Register(ctx context.Context, name, apiAddr, sessionToken string) (string, error) {
	resp, err := c.client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    name,
		ApiAddress:   apiAddr,
		SessionToken: sessionToken,
	})
	if err != nil {
		return "", fmt.Errorf("register failed: %w", err)
	}
	return resp.RegistryUrl, nil
}

func (c *EngineClient) Heartbeat(ctx context.Context, name, sessionToken string) error {
	_, err := c.client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    name,
		SessionToken: sessionToken,
	})
	if err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}
	return nil
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
