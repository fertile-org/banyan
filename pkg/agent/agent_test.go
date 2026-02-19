package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

func TestContainerTracker(t *testing.T) {
	t.Run("add and list", func(t *testing.T) {
		tracker := &containerTracker{}
		tracker.Add("container-1", "task-1")
		tracker.Add("container-2", "task-2")

		list := tracker.List()
		if len(list) != 2 {
			t.Fatalf("expected 2 containers, got %d", len(list))
		}
		if list[0].containerName != "container-1" {
			t.Errorf("expected container-1, got %s", list[0].containerName)
		}
		if list[1].taskID != "task-2" {
			t.Errorf("expected task-2, got %s", list[1].taskID)
		}
	})

	t.Run("list returns copies", func(t *testing.T) {
		tracker := &containerTracker{}
		tracker.Add("container-1", "task-1")

		list1 := tracker.List()
		list1[0].containerName = "modified"

		list2 := tracker.List()
		if list2[0].containerName != "container-1" {
			t.Error("List should return a copy, not a reference")
		}
	})

	t.Run("empty tracker", func(t *testing.T) {
		tracker := &containerTracker{}
		list := tracker.List()
		if len(list) != 0 {
			t.Errorf("expected 0 containers, got %d", len(list))
		}
	})
}

func TestNew(t *testing.T) {
	t.Run("creates agent with session token", func(t *testing.T) {
		a, err := New(&Options{
			NodeName:       "worker-1",
			EngineEndpoint: "localhost:50051",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.sessionToken == "" {
			t.Error("expected non-empty session token")
		}
		if len(a.sessionToken) != 64 { // 32 bytes hex-encoded
			t.Errorf("expected 64-char hex token, got %d chars", len(a.sessionToken))
		}
		if a.opts.NodeName != "worker-1" {
			t.Errorf("expected node name 'worker-1', got %q", a.opts.NodeName)
		}
	})
}

func TestExecuteTask_UnknownType(t *testing.T) {
	task := &types.TaskRecord{Type: "unknown_type"}
	_, err := executeTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for unknown task type")
	}
}

func TestBuildNerdctlRunArgs(t *testing.T) {
	t.Run("basic task", func(t *testing.T) {
		task := &types.TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:alpine",
		}
		args := buildNerdctlRunArgs(task)
		expected := []string{"run", "-d", "--name", "myapp-web-0", "nginx:alpine"}
		if len(args) != len(expected) {
			t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
		}
		for i, exp := range expected {
			if args[i] != exp {
				t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
			}
		}
	})

	t.Run("with ports and env", func(t *testing.T) {
		task := &types.TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:alpine",
			Ports:         []string{"80:80", "443:443"},
			Environment:   []string{"FOO=bar"},
		}
		args := buildNerdctlRunArgs(task)
		// run -d --name myapp-web-0 -p 80:80 -p 443:443 -e FOO=bar nginx:alpine
		if len(args) != 11 {
			t.Fatalf("expected 11 args, got %d: %v", len(args), args)
		}
		if args[4] != "-p" || args[5] != "80:80" {
			t.Errorf("expected -p 80:80, got %s %s", args[4], args[5])
		}
		if args[8] != "-e" || args[9] != "FOO=bar" {
			t.Errorf("expected -e FOO=bar, got %s %s", args[8], args[9])
		}
	})

	t.Run("with command", func(t *testing.T) {
		task := &types.TaskRecord{
			ContainerName: "myapp-worker-0",
			Image:         "python:3",
			Command:       []string{"python", "worker.py"},
		}
		args := buildNerdctlRunArgs(task)
		lastTwo := args[len(args)-2:]
		if lastTwo[0] != "python" || lastTwo[1] != "worker.py" {
			t.Errorf("expected command at end, got %v", lastTwo)
		}
	})
}

func TestProcessTasks(t *testing.T) {
	origExecutor := taskExecutor
	t.Cleanup(func() { taskExecutor = origExecutor })

	t.Run("executes pending tasks and reports results", func(t *testing.T) {
		client, store, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()
		ctx := context.Background()

		// Mock task execution to succeed
		taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
			return &types.TaskResultRecord{ContainerID: "abc123"}, nil
		}

		// Create a pending task
		store.Save(ctx, types.KeyTasks+"worker-1/task-1", &types.TaskRecord{
			ID: "task-1", AgentID: "worker-1", DeploymentID: "deploy-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
			Image: "nginx:alpine", ContainerName: "myapp-web-0",
		})

		a := &Agent{
			opts:       Options{NodeName: "worker-1"},
			client:     client,
			containers: &containerTracker{},
		}

		a.processTasks(ctx)

		// Verify task was reported as completed
		var updated types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"worker-1/task-1", &updated); err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updated.Status != types.StatusCompleted {
			t.Errorf("expected completed, got %s", updated.Status)
		}

		// Verify container was tracked
		tracked := a.containers.List()
		if len(tracked) != 1 {
			t.Fatalf("expected 1 tracked container, got %d", len(tracked))
		}
		if tracked[0].containerName != "myapp-web-0" {
			t.Errorf("expected container name 'myapp-web-0', got %q", tracked[0].containerName)
		}
	})

	t.Run("reports failure when task execution fails", func(t *testing.T) {
		client, store, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()
		ctx := context.Background()

		// Mock task execution to fail
		taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
			return nil, fmt.Errorf("pull failed")
		}

		store.Save(ctx, types.KeyTasks+"worker-1/task-2", &types.TaskRecord{
			ID: "task-2", AgentID: "worker-1", DeploymentID: "deploy-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
			Image: "nginx:alpine", ContainerName: "myapp-api-0",
		})

		a := &Agent{
			opts:       Options{NodeName: "worker-1"},
			client:     client,
			containers: &containerTracker{},
		}

		a.processTasks(ctx)

		// Verify task was reported as failed
		var updated types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"worker-1/task-2", &updated); err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updated.Status != types.StatusFailed {
			t.Errorf("expected failed, got %s", updated.Status)
		}
	})

	t.Run("handles no pending tasks", func(t *testing.T) {
		client, _, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()

		a := &Agent{
			opts:       Options{NodeName: "worker-1"},
			client:     client,
			containers: &containerTracker{},
		}

		// Should not panic with no tasks
		a.processTasks(context.Background())
	})

	t.Run("stop task does not track container", func(t *testing.T) {
		client, store, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()
		ctx := context.Background()

		taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
			return &types.TaskResultRecord{}, nil
		}

		store.Save(ctx, types.KeyTasks+"worker-1/task-3", &types.TaskRecord{
			ID: "task-3", AgentID: "worker-1", DeploymentID: "deploy-1",
			Type: types.TaskTypeStopAndRemove, Status: types.StatusPending,
			ContainerName: "myapp-web-0",
		})

		a := &Agent{
			opts:       Options{NodeName: "worker-1"},
			client:     client,
			containers: &containerTracker{},
		}

		a.processTasks(ctx)

		// Stop tasks should not be tracked
		if len(a.containers.List()) != 0 {
			t.Error("stop task should not add container to tracker")
		}
	})
}

func TestCheckContainerHealth(t *testing.T) {
	origStatusFunc := containerStatusFunc
	t.Cleanup(func() { containerStatusFunc = origStatusFunc })

	t.Run("reports container statuses", func(t *testing.T) {
		client, _, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()

		containerStatusFunc = func(ctx context.Context, name string) string {
			return "running"
		}

		a := &Agent{
			opts:       Options{NodeName: "worker-1"},
			client:     client,
			containers: &containerTracker{},
		}
		a.containers.Add("myapp-web-0", "task-1")
		a.containers.Add("myapp-api-0", "task-2")

		// Should not panic
		a.checkContainerHealth(context.Background())
	})

	t.Run("skips when no containers tracked", func(t *testing.T) {
		a := &Agent{
			containers: &containerTracker{},
		}

		// Should return early without calling client
		a.checkContainerHealth(context.Background())
	})
}

func TestWaitForEngineGRPC(t *testing.T) {
	t.Run("succeeds when engine is ready", func(t *testing.T) {
		client, _, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()

		a := &Agent{client: client}

		err := a.waitForEngineGRPC(context.Background(), 5*time.Second)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("returns error on context cancellation", func(t *testing.T) {
		client, _, cleanup := setupEngineServer(t, "test-pass")
		defer cleanup()

		a := &Agent{client: client}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := a.waitForEngineGRPC(ctx, 5*time.Second)
		if err == nil {
			t.Fatal("expected error on cancelled context")
		}
	})
}

func TestPbTaskToLocal(t *testing.T) {
	pb := &banyanpb.TaskRecord{
		Id:            "task-1",
		DeploymentId:  "deploy-1",
		ServiceName:   "web",
		ReplicaIndex:  2,
		AgentId:       "worker-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusPending,
		Image:         "nginx:alpine",
		ContainerName: "myapp-web-2",
		Ports:         []string{"80:80"},
		Environment:   []string{"ENV=prod"},
		Command:       []string{"nginx", "-g", "daemon off;"},
	}

	task := pbTaskToLocal(pb)

	if task.ID != "task-1" {
		t.Errorf("expected ID task-1, got %s", task.ID)
	}
	if task.ReplicaIndex != 2 {
		t.Errorf("expected ReplicaIndex 2, got %d", task.ReplicaIndex)
	}
	if task.Image != "nginx:alpine" {
		t.Errorf("expected Image nginx:alpine, got %s", task.Image)
	}
	if len(task.Ports) != 1 || task.Ports[0] != "80:80" {
		t.Errorf("expected Ports [80:80], got %v", task.Ports)
	}
	if len(task.Command) != 3 {
		t.Errorf("expected 3 command args, got %d", len(task.Command))
	}
}

func TestExecuteCreateAndStart(t *testing.T) {
	origRunner := commandRunner
	origIDGetter := containerIDGetter
	t.Cleanup(func() {
		commandRunner = origRunner
		containerIDGetter = origIDGetter
	})

	commandRunner = func(ctx context.Context, name string, args ...string) error {
		return nil
	}
	containerIDGetter = func(ctx context.Context, containerName string) (string, error) {
		return "abc123", nil
	}

	task := &types.TaskRecord{
		Image:         "nginx:alpine",
		ContainerName: "myapp-web-0",
	}

	result, err := executeCreateAndStart(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContainerID != "abc123" {
		t.Errorf("expected container ID 'abc123', got %q", result.ContainerID)
	}
}

func TestExecuteCreateAndStart_PullFails(t *testing.T) {
	origRunner := commandRunner
	origIDGetter := containerIDGetter
	t.Cleanup(func() {
		commandRunner = origRunner
		containerIDGetter = origIDGetter
	})

	callCount := 0
	commandRunner = func(ctx context.Context, name string, args ...string) error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("pull failed: image not found")
		}
		return nil
	}
	containerIDGetter = func(ctx context.Context, containerName string) (string, error) {
		return "abc123", nil
	}

	task := &types.TaskRecord{
		Image:         "nonexistent:latest",
		ContainerName: "myapp-web-0",
	}

	_, err := executeCreateAndStart(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when pull fails")
	}
	if !strings.Contains(err.Error(), "failed to pull image") {
		t.Errorf("expected 'failed to pull image' in error, got: %v", err)
	}
}

func TestExecuteCreateAndStart_RunFails(t *testing.T) {
	origRunner := commandRunner
	origIDGetter := containerIDGetter
	t.Cleanup(func() {
		commandRunner = origRunner
		containerIDGetter = origIDGetter
	})

	callCount := 0
	commandRunner = func(ctx context.Context, name string, args ...string) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("port already in use")
		}
		return nil
	}
	containerIDGetter = func(ctx context.Context, containerName string) (string, error) {
		return "abc123", nil
	}

	task := &types.TaskRecord{
		Image:         "nginx:alpine",
		ContainerName: "myapp-web-0",
	}

	_, err := executeCreateAndStart(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when run fails")
	}
	if !strings.Contains(err.Error(), "failed to start container") {
		t.Errorf("expected 'failed to start container' in error, got: %v", err)
	}
}

func TestExecuteCreateAndStart_GetIDFails(t *testing.T) {
	origRunner := commandRunner
	origIDGetter := containerIDGetter
	t.Cleanup(func() {
		commandRunner = origRunner
		containerIDGetter = origIDGetter
	})

	commandRunner = func(ctx context.Context, name string, args ...string) error {
		return nil
	}
	containerIDGetter = func(ctx context.Context, containerName string) (string, error) {
		return "", fmt.Errorf("inspect failed")
	}

	task := &types.TaskRecord{
		Image:         "nginx:alpine",
		ContainerName: "myapp-web-0",
	}

	result, err := executeCreateAndStart(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContainerID != "unknown" {
		t.Errorf("expected container ID 'unknown' when getID fails, got %q", result.ContainerID)
	}
}

func TestExecuteStopAndRemove(t *testing.T) {
	origRemover := containerRemover
	t.Cleanup(func() { containerRemover = origRemover })

	containerRemover = func(ctx context.Context, containerName string) error {
		return nil
	}

	task := &types.TaskRecord{
		ContainerName: "myapp-web-0",
	}

	result, err := executeStopAndRemove(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestExecuteStopAndRemove_Fails(t *testing.T) {
	origRemover := containerRemover
	t.Cleanup(func() { containerRemover = origRemover })

	containerRemover = func(ctx context.Context, containerName string) error {
		return fmt.Errorf("failed to remove container: permission denied")
	}

	task := &types.TaskRecord{
		ContainerName: "myapp-web-0",
	}

	_, err := executeStopAndRemove(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when remove fails")
	}
	if !strings.Contains(err.Error(), "failed to remove container") {
		t.Errorf("expected 'failed to remove container' in error, got: %v", err)
	}
}

func TestExecuteTask_CreateAndStart(t *testing.T) {
	origRunner := commandRunner
	origIDGetter := containerIDGetter
	t.Cleanup(func() {
		commandRunner = origRunner
		containerIDGetter = origIDGetter
	})

	commandRunner = func(ctx context.Context, name string, args ...string) error {
		return nil
	}
	containerIDGetter = func(ctx context.Context, containerName string) (string, error) {
		return "container-id-123", nil
	}

	task := &types.TaskRecord{
		Type:          types.TaskTypeCreateAndStart,
		Image:         "nginx:alpine",
		ContainerName: "myapp-web-0",
	}

	result, err := executeTask(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ContainerID != "container-id-123" {
		t.Errorf("expected container ID 'container-id-123', got %q", result.ContainerID)
	}
}

func TestExecuteTask_StopAndRemove(t *testing.T) {
	origRemover := containerRemover
	t.Cleanup(func() { containerRemover = origRemover })

	containerRemover = func(ctx context.Context, containerName string) error {
		return nil
	}

	task := &types.TaskRecord{
		Type:          types.TaskTypeStopAndRemove,
		ContainerName: "myapp-web-0",
	}

	result, err := executeTask(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestRunCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("success with echo", func(t *testing.T) {
		err := runCommand(ctx, "echo", "hello")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("failure with false", func(t *testing.T) {
		err := runCommand(ctx, "false")
		if err == nil {
			t.Fatal("expected error from 'false' command")
		}
	})

	t.Run("stderr included in error", func(t *testing.T) {
		err := runCommand(ctx, "sh", "-c", "echo 'error msg' >&2 && exit 1")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "error msg") {
			t.Errorf("expected stderr in error message, got: %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		err := runCommand(cancelCtx, "sleep", "10")
		if err == nil {
			t.Fatal("expected error on cancelled context")
		}
	})
}

func TestProcessTasks_PollError(t *testing.T) {
	// When PollTasks fails (e.g. server stopped), processTasks should return early.
	client, _, cleanup := setupEngineServer(t, "test-pass")
	cleanup() // Stop server immediately to trigger error

	a := &Agent{
		opts:       Options{NodeName: "worker-1"},
		client:     client,
		containers: &containerTracker{},
	}

	// Should not panic; just returns early on poll error
	a.processTasks(context.Background())
}

func TestProcessTasks_NilResult(t *testing.T) {
	origExecutor := taskExecutor
	t.Cleanup(func() { taskExecutor = origExecutor })

	client, store, cleanup := setupEngineServer(t, "test-pass")
	defer cleanup()
	ctx := context.Background()

	// Mock task execution to return nil result (no container ID)
	taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
		return nil, nil
	}

	store.Save(ctx, types.KeyTasks+"worker-1/task-nil", &types.TaskRecord{
		ID: "task-nil", AgentID: "worker-1", DeploymentID: "deploy-1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusPending,
		ContainerName: "myapp-web-0",
	})

	a := &Agent{
		opts:       Options{NodeName: "worker-1"},
		client:     client,
		containers: &containerTracker{},
	}

	a.processTasks(ctx)

	var updated types.TaskRecord
	if err := store.Get(ctx, types.KeyTasks+"worker-1/task-nil", &updated); err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updated.Status != types.StatusCompleted {
		t.Errorf("expected completed, got %s", updated.Status)
	}
}

func TestCheckContainerHealth_ReportError(t *testing.T) {
	origStatusFunc := containerStatusFunc
	t.Cleanup(func() { containerStatusFunc = origStatusFunc })

	// Use a stopped server so ReportContainerHealth fails
	client, _, cleanup := setupEngineServer(t, "test-pass")
	cleanup() // Stop server immediately

	containerStatusFunc = func(ctx context.Context, name string) string {
		return "running"
	}

	a := &Agent{
		opts:       Options{NodeName: "worker-1"},
		client:     client,
		containers: &containerTracker{},
	}
	a.containers.Add("myapp-web-0", "task-1")

	// Should not panic; just prints warning
	a.checkContainerHealth(context.Background())
}

func TestWaitForEngineGRPC_Timeout(t *testing.T) {
	// Use a stopped server so Health always fails
	client, _, cleanup := setupEngineServer(t, "test-pass")
	cleanup()

	a := &Agent{client: client}

	err := a.waitForEngineGRPC(context.Background(), 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout in error, got: %v", err)
	}
}

func TestRemoveContainer_CommandNotFound(t *testing.T) {
	// removeContainer calls "nerdctl rm -f" which is not available in test environment.
	// When the command is not found, it should return an error (not panic).
	err := removeContainer(context.Background(), "test-container")
	if err == nil {
		// If nerdctl happens to be installed and the container doesn't exist,
		// it may or may not return an error (the "not found" path returns nil).
		// So we can't assert error is non-nil — just verify no panic.
		return
	}
	// If we get here, the error should indicate failure
	if !strings.Contains(err.Error(), "failed to remove container") {
		t.Errorf("expected 'failed to remove container' in error, got: %v", err)
	}
}

func TestRemoveContainer_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// With a cancelled context, the command should fail
	err := removeContainer(ctx, "test-container")
	// Either the command fails to start or returns an error; no panic expected.
	_ = err
}

func TestProcessTasks_ReportRunningFails(t *testing.T) {
	origExecutor := taskExecutor
	t.Cleanup(func() { taskExecutor = origExecutor })

	// Use failing report server where ReportTaskResult always returns an error
	client, store, cleanup := setupFailingReportServer(t, "test-pass")
	defer cleanup()
	ctx := context.Background()

	// Mock task execution to succeed
	taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
		return &types.TaskResultRecord{ContainerID: "abc123"}, nil
	}

	store.Save(ctx, types.KeyTasks+"worker-1/task-fr1", &types.TaskRecord{
		ID: "task-fr1", AgentID: "worker-1", DeploymentID: "deploy-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		Image: "nginx:alpine", ContainerName: "myapp-web-0",
	})

	a := &Agent{
		opts:       Options{NodeName: "worker-1"},
		client:     client,
		containers: &containerTracker{},
	}

	// Should not panic; the WARNING prints cover the error paths for
	// reporting "running" and "completed" statuses.
	a.processTasks(ctx)

	// Container should still be tracked despite report errors
	tracked := a.containers.List()
	if len(tracked) != 1 {
		t.Fatalf("expected 1 tracked container, got %d", len(tracked))
	}
}

func TestProcessTasks_ReportFailureFails(t *testing.T) {
	origExecutor := taskExecutor
	t.Cleanup(func() { taskExecutor = origExecutor })

	// Use failing report server where ReportTaskResult always returns an error
	client, store, cleanup := setupFailingReportServer(t, "test-pass")
	defer cleanup()
	ctx := context.Background()

	// Mock task execution to fail
	taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
		return nil, fmt.Errorf("execution failed")
	}

	store.Save(ctx, types.KeyTasks+"worker-1/task-fr2", &types.TaskRecord{
		ID: "task-fr2", AgentID: "worker-1", DeploymentID: "deploy-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		Image: "nginx:alpine", ContainerName: "myapp-api-0",
	})

	a := &Agent{
		opts:       Options{NodeName: "worker-1"},
		client:     client,
		containers: &containerTracker{},
	}

	// Should not panic; the WARNING prints cover the error path for
	// reporting "failed" status when ReportTaskResult itself fails.
	a.processTasks(ctx)

	// Container should not be tracked since the task failed
	if len(a.containers.List()) != 0 {
		t.Error("failed task should not add container to tracker")
	}
}

func TestCmdReadCloser(t *testing.T) {
	t.Run("read and close with real command", func(t *testing.T) {
		cmd := exec.Command("echo", "hello world")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatalf("failed to create stdout pipe: %v", err)
		}

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start command: %v", err)
		}

		rc := &cmdReadCloser{cmd: cmd, reader: stdout}

		// Read output
		buf := make([]byte, 100)
		n, _ := rc.Read(buf)
		if n == 0 {
			t.Error("expected some data from echo command")
		}
		output := strings.TrimSpace(string(buf[:n]))
		if output != "hello world" {
			t.Errorf("expected 'hello world', got %q", output)
		}

		// Close should not panic
		if err := rc.Close(); err != nil {
			t.Errorf("unexpected error from Close: %v", err)
		}
	})

	t.Run("close with long-running command", func(t *testing.T) {
		cmd := exec.Command("sleep", "60")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatalf("failed to create stdout pipe: %v", err)
		}

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start command: %v", err)
		}

		rc := &cmdReadCloser{cmd: cmd, reader: stdout}

		// Close should kill the process and not hang
		if err := rc.Close(); err != nil {
			t.Errorf("unexpected error from Close: %v", err)
		}
	})
}
