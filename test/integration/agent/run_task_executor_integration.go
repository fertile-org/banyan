//go:build ignore

// Integration test for Agent Task Executor
// Tests the full flow: Task Executor → Container Service → ContainerdRuntime → containerd
//
// Prerequisites:
// - containerd running
// - Run with: sudo go run ./test/integration/agent/run_task_executor_integration.go

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/container/adapters"
	containerdomain "github.com/fertile-org/banyan/pkg/agent/container/domain"
	containeruc "github.com/fertile-org/banyan/pkg/agent/container/usecases"
	taskadapters "github.com/fertile-org/banyan/pkg/agent/task/adapters"
	"github.com/fertile-org/banyan/pkg/agent/task/domain"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/outbound"
	taskuc "github.com/fertile-org/banyan/pkg/agent/task/usecases"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

const (
	testContainerName = "test-task-executor-container"
	testImage         = "docker.io/library/alpine:latest"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "All Task Executor integration tests passed!")
	} else {
		p.Result(false, "Task Executor integration tests failed")
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Task Executor Integration Test")

	// Step 1: Check containerd is running
	p.Step("Checking containerd availability")
	if err := helpers.CheckContainerdAvailable(ctx); err != nil {
		p.Error(fmt.Sprintf("containerd not available: %v", err))
		p.Info("Start containerd: sudo systemctl start containerd")
		return 1
	}
	p.Success("containerd is available")

	// Step 2: Create real services
	p.Step("Creating real service chain")

	// Create containerd runtime
	containerdRuntime, err := adapters.NewContainerdRuntime(adapters.ContainerdRuntimeConfig{
		Address:   "/run/containerd/containerd.sock",
		Namespace: "banyan-test",
	})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create containerd runtime: %v", err))
		return 1
	}

	// Create image registry using the same containerd client
	imageRegistry := adapters.NewContainerdImageRegistry(
		containerdRuntime.Client(),
		containerdRuntime.Namespace(),
	)

	// Create container use case (implements ContainerService)
	containerService := containeruc.NewContainerUseCase(
		containerdRuntime,
		nil, // network connector - not needed for this test
		nil, // event publisher - not needed for this test
		imageRegistry,
	)

	// Create Task Executor with real container service
	// All adapters follow consistent naming: *Adapter suffix
	taskStoreAdapter := taskadapters.NewMemoryTaskStoreAdapter()
	eventEmitterAdapter := taskadapters.NewMemoryEventEmitterAdapter()
	containerAdapter := taskadapters.NewContainerServiceAdapter(containerService)

	taskExecutor := taskuc.NewTaskUseCase(
		containerAdapter,
		nil, // network service adapter
		nil, // security service adapter
		nil, // health service adapter
		taskStoreAdapter,
		eventEmitterAdapter,
	)

	p.Success("Service chain created")

	// Step 3: Test container.create task
	p.Step("Testing container.create task")

	createTask := &domain.Task{
		ID:      domain.NewTaskID(),
		Type:    domain.TaskTypeContainerCreate,
		Timeout: 60 * time.Second,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				Name:    testContainerName,
				Image:   testImage,
				Command: []string{"sleep", "300"},
			},
		},
	}

	result, err := taskExecutor.SubmitTask(ctx, createTask)
	if err != nil {
		p.Error(fmt.Sprintf("container.create task failed: %v", err))
		return 1
	}

	if !result.Success {
		p.Error(fmt.Sprintf("container.create returned failure: %s", result.Error))
		return 1
	}

	// The output is *outbound.ContainerInfo
	containerInfo, ok := result.Output.(*outbound.ContainerInfo)
	if !ok {
		p.Error(fmt.Sprintf("container.create returned unexpected output type: %T", result.Output))
		return 1
	}

	p.Success(fmt.Sprintf("Container created: ID=%s, Name=%s", containerInfo.ID, containerInfo.Name))

	// Step 4: Verify container exists in containerd
	p.Step("Verifying container exists in containerd")

	info, err := containerAdapter.GetContainer(ctx, containerInfo.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get container: %v", err))
		cleanup(ctx, containerService, containerInfo.ID, p)
		return 1
	}

	p.Success(fmt.Sprintf("Container verified: State=%s", info.State))

	// Step 5: Test container.start task
	p.Step("Testing container.start task")

	startTask := &domain.Task{
		ID:      domain.NewTaskID(),
		Type:    domain.TaskTypeContainerStart,
		Timeout: 30 * time.Second,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: containerInfo.ID,
			},
		},
	}

	result, err = taskExecutor.SubmitTask(ctx, startTask)
	if err != nil {
		p.Error(fmt.Sprintf("container.start task failed: %v", err))
		cleanup(ctx, containerService, containerInfo.ID, p)
		return 1
	}

	if !result.Success {
		p.Error(fmt.Sprintf("container.start returned failure: %s", result.Error))
		cleanup(ctx, containerService, containerInfo.ID, p)
		return 1
	}

	p.Success("Container started")

	// Step 6: Verify container is running
	p.Step("Verifying container is running")

	info, err = containerAdapter.GetContainer(ctx, containerInfo.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get container: %v", err))
		cleanup(ctx, containerService, containerInfo.ID, p)
		return 1
	}

	if info.State != "running" {
		p.Error(fmt.Sprintf("Expected state 'running', got '%s'", info.State))
		cleanup(ctx, containerService, containerInfo.ID, p)
		return 1
	}

	p.Success("Container is running")

	// Step 7: Test container.stop task
	p.Step("Testing container.stop task")

	stopTask := &domain.Task{
		ID:      domain.NewTaskID(),
		Type:    domain.TaskTypeContainerStop,
		Timeout: 30 * time.Second,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: containerInfo.ID,
				Timeout:     10 * time.Second,
			},
		},
	}

	result, err = taskExecutor.SubmitTask(ctx, stopTask)
	if err != nil {
		p.Error(fmt.Sprintf("container.stop task failed: %v", err))
		cleanup(ctx, containerService, containerInfo.ID, p)
		return 1
	}

	p.Success("Container stopped")

	// Step 8: Test container.remove task
	p.Step("Testing container.remove task")

	removeTask := &domain.Task{
		ID:      domain.NewTaskID(),
		Type:    domain.TaskTypeContainerRemove,
		Timeout: 30 * time.Second,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: containerInfo.ID,
			},
		},
	}

	result, err = taskExecutor.SubmitTask(ctx, removeTask)
	if err != nil {
		p.Error(fmt.Sprintf("container.remove task failed: %v", err))
		return 1
	}

	p.Success("Container removed")

	// Step 9: Verify cleanup
	p.Step("Verifying container no longer exists")

	_, err = containerAdapter.GetContainer(ctx, containerInfo.ID)
	if err == nil {
		p.Error("Container should not exist after removal")
		return 1
	}

	p.Success("Container cleanup verified")

	return 0
}

func cleanup(ctx context.Context, service *containeruc.ContainerUseCase, containerID string, p *helpers.Printer) {
	p.Step("Cleaning up test container")

	// Stop first (ignore errors)
	_ = service.StopContainer(ctx, containerdomain.ContainerID(containerID), 5*time.Second)

	// Then remove
	if err := service.RemoveContainer(ctx, containerdomain.ContainerID(containerID), true); err != nil {
		p.Warning(fmt.Sprintf("Cleanup warning: %v", err))
	} else {
		p.Success("Cleanup completed")
	}
}
