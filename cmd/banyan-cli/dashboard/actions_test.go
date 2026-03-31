package dashboard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"google.golang.org/grpc"
)

// mockEngineClient implements banyanpb.EngineServiceClient for testing.
type mockEngineClient struct {
	banyanpb.EngineServiceClient
	stopTaskResp *banyanpb.StopTaskResponse
	stopTaskErr  error
	scaleResp    *banyanpb.ScaleResponse
	scaleErr     error
	downResp     *banyanpb.DownRPCResponse
	downErr      error
}

func (m *mockEngineClient) StopTask(_ context.Context, _ *banyanpb.StopTaskRequest, _ ...grpc.CallOption) (*banyanpb.StopTaskResponse, error) {
	return m.stopTaskResp, m.stopTaskErr
}

func (m *mockEngineClient) Scale(_ context.Context, _ *banyanpb.ScaleRequest, _ ...grpc.CallOption) (*banyanpb.ScaleResponse, error) {
	return m.scaleResp, m.scaleErr
}

func (m *mockEngineClient) Down(_ context.Context, _ *banyanpb.DownRPCRequest, _ ...grpc.CallOption) (*banyanpb.DownRPCResponse, error) {
	return m.downResp, m.downErr
}

func TestStopContainerCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &mockEngineClient{
			stopTaskResp: &banyanpb.StopTaskResponse{
				StopTaskId: "task-1-stop",
				Status:     "stopping",
			},
		}
		cmd := stopContainerCmd(client, "task-1", "agent-1")
		msg := cmd()
		result, ok := msg.(actionResultMsg)
		if !ok {
			t.Fatalf("expected actionResultMsg, got %T", msg)
		}
		if !result.success {
			t.Errorf("expected success, got: %s", result.message)
		}
		if !strings.Contains(result.message, "task-1-stop") {
			t.Errorf("message = %q, should contain stop task ID", result.message)
		}
	})

	t.Run("error", func(t *testing.T) {
		client := &mockEngineClient{
			stopTaskErr: fmt.Errorf("not found"),
		}
		cmd := stopContainerCmd(client, "task-1", "agent-1")
		msg := cmd()
		result, ok := msg.(actionResultMsg)
		if !ok {
			t.Fatalf("expected actionResultMsg, got %T", msg)
		}
		if result.success {
			t.Error("expected failure")
		}
		if !strings.Contains(result.message, "not found") {
			t.Errorf("message = %q, should contain error", result.message)
		}
	})
}

func TestScaleDeploymentCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &mockEngineClient{
			scaleResp: &banyanpb.ScaleResponse{
				DeploymentId: "myapp-1",
			},
		}
		cmd := scaleDeploymentCmd(client, "myapp", "web", 3)
		msg := cmd()
		result, ok := msg.(actionResultMsg)
		if !ok {
			t.Fatalf("expected actionResultMsg, got %T", msg)
		}
		if !result.success {
			t.Errorf("expected success, got: %s", result.message)
		}
	})

	t.Run("error", func(t *testing.T) {
		client := &mockEngineClient{
			scaleErr: fmt.Errorf("no agents"),
		}
		cmd := scaleDeploymentCmd(client, "myapp", "web", 3)
		msg := cmd()
		result, ok := msg.(actionResultMsg)
		if !ok {
			t.Fatalf("expected actionResultMsg, got %T", msg)
		}
		if result.success {
			t.Error("expected failure")
		}
	})
}

func TestTeardownDeploymentCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		client := &mockEngineClient{
			downResp: &banyanpb.DownRPCResponse{TaskCount: 2},
		}
		cmd := teardownDeploymentCmd(client, "myapp", nil)
		msg := cmd()
		result, ok := msg.(actionResultMsg)
		if !ok {
			t.Fatalf("expected actionResultMsg, got %T", msg)
		}
		if !result.success {
			t.Errorf("expected success, got: %s", result.message)
		}
		if !strings.Contains(result.message, "2 tasks") {
			t.Errorf("message = %q, should contain task count", result.message)
		}
	})

	t.Run("error", func(t *testing.T) {
		client := &mockEngineClient{
			downErr: fmt.Errorf("deployment not found"),
		}
		cmd := teardownDeploymentCmd(client, "myapp", nil)
		msg := cmd()
		result, ok := msg.(actionResultMsg)
		if !ok {
			t.Fatalf("expected actionResultMsg, got %T", msg)
		}
		if result.success {
			t.Error("expected failure")
		}
	})
}
