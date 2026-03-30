package dashboard

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// actionResultMsg is sent when an async action completes.
type actionResultMsg struct {
	message string
	success bool
}

// stopContainerCmd calls StopTask RPC and returns the result as a tea.Msg.
func stopContainerCmd(client banyanpb.EngineServiceClient, taskID, agentID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.StopTask(ctx, &banyanpb.StopTaskRequest{
			TaskId:  taskID,
			AgentId: agentID,
		})
		if err != nil {
			return actionResultMsg{success: false, message: fmt.Sprintf("Failed: %v", err)}
		}
		return actionResultMsg{success: true, message: fmt.Sprintf("Stopping (%s)", resp.StopTaskId)}
	}
}

// scaleDeploymentCmd calls Scale RPC.
func scaleDeploymentCmd(client banyanpb.EngineServiceClient, name, service string, count int32) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.Scale(ctx, &banyanpb.ScaleRequest{
			Name:     name,
			Replicas: map[string]int32{service: count},
		})
		if err != nil {
			return actionResultMsg{success: false, message: fmt.Sprintf("Scale failed: %v", err)}
		}
		return actionResultMsg{success: true, message: fmt.Sprintf("Scaled %s (deployment %s)", service, resp.DeploymentId)}
	}
}

// teardownDeploymentCmd calls Down RPC.
func teardownDeploymentCmd(client banyanpb.EngineServiceClient, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.Down(ctx, &banyanpb.DownRPCRequest{
			Name: name,
		})
		if err != nil {
			return actionResultMsg{success: false, message: fmt.Sprintf("Teardown failed: %v", err)}
		}
		return actionResultMsg{success: true, message: fmt.Sprintf("Teardown started (%d tasks)", resp.TaskCount)}
	}
}
