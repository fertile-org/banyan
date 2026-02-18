package cmd

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// engineClient wraps the gRPC connection to the engine.
type engineClient struct {
	conn   *grpc.ClientConn
	client banyanpb.EngineServiceClient
}

// newEngineClient dials the engine gRPC server with password credentials.
func newEngineClient(engineAddr, password string) (*engineClient, error) {
	conn, err := grpc.NewClient(engineAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: password}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine at %s: %w", engineAddr, err)
	}

	return &engineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}, nil
}

func (c *engineClient) Register(ctx context.Context, name, apiAddr, sessionToken string) (string, error) {
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

func (c *engineClient) Heartbeat(ctx context.Context, name, sessionToken string) error {
	_, err := c.client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    name,
		SessionToken: sessionToken,
	})
	if err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}
	return nil
}

func (c *engineClient) PollTasks(ctx context.Context, name string) ([]*banyanpb.TaskRecord, error) {
	resp, err := c.client.PollTasks(ctx, &banyanpb.PollTasksRequest{
		AgentName: name,
	})
	if err != nil {
		return nil, fmt.Errorf("poll tasks failed: %w", err)
	}
	return resp.Tasks, nil
}

func (c *engineClient) ReportTaskResult(ctx context.Context, taskID, agentID, status, errMsg, containerName string, result *banyanpb.TaskResult) error {
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

func (c *engineClient) ReportContainerHealth(ctx context.Context, agentName string, containers []*banyanpb.ContainerStatus) error {
	_, err := c.client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName:  agentName,
		Containers: containers,
	})
	if err != nil {
		return fmt.Errorf("report container health failed: %w", err)
	}
	return nil
}

func (c *engineClient) Health(ctx context.Context) error {
	_, err := c.client.Health(ctx, &banyanpb.HealthRequest{})
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

func (c *engineClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}
