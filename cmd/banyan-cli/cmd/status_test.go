package cmd

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"google.golang.org/grpc"
)

func TestRunStatus_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = "/tmp/nonexistent-cli-config.yaml"

	err := runStatus(statusCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunStatus_WithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	err := runStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestGroupTaskInfoByService(t *testing.T) {
	t.Run("groups by service name", func(t *testing.T) {
		tasks := []*taskInfoForDisplay{
			{ServiceName: "web", ContainerName: "myapp-web-0"},
			{ServiceName: "api", ContainerName: "myapp-api-0"},
			{ServiceName: "web", ContainerName: "myapp-web-1"},
		}
		grouped := groupTaskInfoByService(tasks)
		if len(grouped) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(grouped))
		}
		if len(grouped["web"]) != 2 {
			t.Errorf("expected 2 web tasks, got %d", len(grouped["web"]))
		}
		if len(grouped["api"]) != 1 {
			t.Errorf("expected 1 api task, got %d", len(grouped["api"]))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		grouped := groupTaskInfoByService(nil)
		if len(grouped) != 0 {
			t.Errorf("expected 0 groups, got %d", len(grouped))
		}
	})
}

func TestSortedServiceNamesFromInfo(t *testing.T) {
	t.Run("sorted output", func(t *testing.T) {
		grouped := map[string][]*taskInfoForDisplay{
			"web": {{ServiceName: "web"}},
			"api": {{ServiceName: "api"}},
			"db":  {{ServiceName: "db"}},
		}
		names := sortedServiceNamesFromInfo(grouped)
		expected := []string{"api", "db", "web"}
		if len(names) != len(expected) {
			t.Fatalf("expected %d names, got %d", len(expected), len(names))
		}
		for i, exp := range expected {
			if names[i] != exp {
				t.Errorf("index %d: expected %q, got %q", i, exp, names[i])
			}
		}
	})

	t.Run("empty map", func(t *testing.T) {
		names := sortedServiceNamesFromInfo(map[string][]*taskInfoForDisplay{})
		if len(names) != 0 {
			t.Errorf("expected 0 names, got %d", len(names))
		}
	})
}

// richStatusServer returns status with multiple agents, deployments, and varied container states.
type richStatusServer struct {
	banyanpb.UnimplementedEngineServiceServer
}

func (s *richStatusServer) GetStatus(_ context.Context, _ *banyanpb.GetStatusRequest) (*banyanpb.GetStatusResponse, error) {
	return &banyanpb.GetStatusResponse{
		Agents: []*banyanpb.AgentInfo{
			{Name: "worker-1", Status: "ready", LastSeenUnix: time.Now().Unix()},
			{Name: "worker-2", Status: "ready", LastSeenUnix: time.Now().Add(-30 * time.Second).Unix()},
		},
		Deployments: []*banyanpb.DeploymentInfo{
			{
				Name: "rich-app", Status: types.StatusRunning, Healthy: 2, Total: 3,
				Tasks: []*banyanpb.TaskInfo{
					{
						ServiceName: "web", ContainerName: "rich-app-web-0", AgentId: "worker-1",
						Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning,
						ContainerCheckedAtUnix: time.Now().Add(-5 * time.Second).Unix(),
					},
					{
						ServiceName: "web", ContainerName: "rich-app-web-1", AgentId: "worker-2",
						Type: types.TaskTypeCreateAndStart, ContainerStatus: "", // empty → "pending" fallback
					},
					{
						ServiceName: "api", ContainerName: "rich-app-api-0", AgentId: "worker-1",
						Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning,
						ContainerCheckedAtUnix: time.Now().Add(-10 * time.Second).Unix(),
					},
					{
						ServiceName: "web", ContainerName: "rich-app-web-0", AgentId: "worker-1",
						Type: types.TaskTypeStopAndRemove, // should be filtered out
					},
				},
			},
		},
	}, nil
}

func (s *richStatusServer) Health(_ context.Context, _ *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

func TestRunStatus_WithRichData(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &richStatusServer{})
	go srv.Serve(lis)
	t.Cleanup(func() { srv.Stop() })

	setupCLITestConfig(t, lis.Addr().String())

	err = runStatus(statusCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
