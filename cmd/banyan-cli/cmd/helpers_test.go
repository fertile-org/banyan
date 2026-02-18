package cmd

import (
	"testing"
)

func TestBuildServiceRecords(t *testing.T) {
	t.Run("defaults replicas to 1 when zero", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"web": {Image: "nginx:latest", Replicas: 0},
		}
		services := buildServiceRecords(manifest)
		if services["web"].Replicas != 1 {
			t.Errorf("expected replicas=1, got %d", services["web"].Replicas)
		}
	})

	t.Run("preserves explicit replicas", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"web": {Image: "nginx:latest", Replicas: 3},
		}
		services := buildServiceRecords(manifest)
		if services["web"].Replicas != 3 {
			t.Errorf("expected replicas=3, got %d", services["web"].Replicas)
		}
	})

	t.Run("maps all fields correctly", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"api": {
				Image:     "my-api:v1",
				Replicas:  2,
				Ports:     []string{"8080:80"},
				Env:       []string{"DB=postgres"},
				Command:   []string{"serve"},
				DependsOn: []string{"db"},
			},
		}
		services := buildServiceRecords(manifest)
		svc := services["api"]

		if svc.Image != "my-api:v1" {
			t.Errorf("expected image my-api:v1, got %s", svc.Image)
		}
		if len(svc.Ports) != 1 || svc.Ports[0] != "8080:80" {
			t.Errorf("unexpected ports: %v", svc.Ports)
		}
		if len(svc.Environment) != 1 || svc.Environment[0] != "DB=postgres" {
			t.Errorf("unexpected env: %v", svc.Environment)
		}
		if len(svc.Command) != 1 || svc.Command[0] != "serve" {
			t.Errorf("unexpected command: %v", svc.Command)
		}
		if len(svc.DependsOn) != 1 || svc.DependsOn[0] != "db" {
			t.Errorf("unexpected depends_on: %v", svc.DependsOn)
		}
	})
}

func TestBuildTasksForDeployment(t *testing.T) {
	agents := []NodeRecord{
		{Name: "agent-1"},
		{Name: "agent-2"},
	}

	t.Run("creates correct number of tasks", func(t *testing.T) {
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"web": {Image: "nginx", Replicas: 3},
			},
		}
		tasks := buildTasksForDeployment(deployment, agents)
		if len(tasks) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(tasks))
		}
	})

	t.Run("round-robins across agents", func(t *testing.T) {
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"web": {Image: "nginx", Replicas: 4},
			},
		}
		tasks := buildTasksForDeployment(deployment, agents)

		// With 4 tasks and 2 agents, each agent should get 2
		agentCounts := map[string]int{}
		for _, task := range tasks {
			agentCounts[task.AgentID]++
		}
		if agentCounts["agent-1"] != 2 || agentCounts["agent-2"] != 2 {
			t.Errorf("expected even distribution, got %v", agentCounts)
		}
	})

	t.Run("sets correct task fields", func(t *testing.T) {
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"web": {
					Image:       "nginx:latest",
					Replicas:    1,
					Ports:       []string{"80:80"},
					Environment: []string{"FOO=bar"},
					Command:     []string{"serve"},
				},
			},
		}
		tasks := buildTasksForDeployment(deployment, agents)
		task := tasks[0]

		if task.DeploymentID != "deploy-1" {
			t.Errorf("unexpected deployment_id: %s", task.DeploymentID)
		}
		if task.ServiceName != "web" {
			t.Errorf("unexpected service_name: %s", task.ServiceName)
		}
		if task.Type != taskTypeCreateAndStart {
			t.Errorf("unexpected type: %s", task.Type)
		}
		if task.Status != statusPending {
			t.Errorf("unexpected status: %s", task.Status)
		}
		if task.Image != "nginx:latest" {
			t.Errorf("unexpected image: %s", task.Image)
		}
		if task.ContainerName != "myapp-web-0" {
			t.Errorf("unexpected container_name: %s", task.ContainerName)
		}
		if len(task.Ports) != 1 || task.Ports[0] != "80:80" {
			t.Errorf("unexpected ports: %v", task.Ports)
		}
	})

	t.Run("handles multiple services", func(t *testing.T) {
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"web": {Image: "nginx", Replicas: 2},
				"api": {Image: "myapi", Replicas: 1},
			},
		}
		tasks := buildTasksForDeployment(deployment, agents)
		if len(tasks) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(tasks))
		}
	})
}

func TestDetermineDeploymentStatus(t *testing.T) {
	t.Run("returns empty when no tasks", func(t *testing.T) {
		status, _ := determineDeploymentStatus(0, 0, 0, "")
		if status != "" {
			t.Errorf("expected empty status, got %s", status)
		}
	})

	t.Run("returns running when all completed", func(t *testing.T) {
		status, errMsg := determineDeploymentStatus(3, 3, 0, "")
		if status != statusRunning {
			t.Errorf("expected running, got %s", status)
		}
		if errMsg != "" {
			t.Errorf("expected no error, got %s", errMsg)
		}
	})

	t.Run("returns failed when any task failed", func(t *testing.T) {
		status, errMsg := determineDeploymentStatus(3, 1, 1, "pull failed")
		if status != statusFailed {
			t.Errorf("expected failed, got %s", status)
		}
		if errMsg == "" {
			t.Error("expected error message")
		}
	})

	t.Run("returns empty when still in progress", func(t *testing.T) {
		status, _ := determineDeploymentStatus(3, 1, 0, "")
		if status != "" {
			t.Errorf("expected empty (in progress), got %s", status)
		}
	})

	t.Run("failed takes priority over completed", func(t *testing.T) {
		status, _ := determineDeploymentStatus(3, 2, 1, "timeout")
		if status != statusFailed {
			t.Errorf("expected failed, got %s", status)
		}
	})
}

func TestBuildNerdctlRunArgs(t *testing.T) {
	t.Run("basic task with no extras", func(t *testing.T) {
		task := &TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:latest",
		}
		args := buildNerdctlRunArgs(task)
		expected := []string{"run", "-d", "--name", "myapp-web-0", "nginx:latest"}
		assertSliceEqual(t, expected, args)
	})

	t.Run("includes port mappings", func(t *testing.T) {
		task := &TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:latest",
			Ports:         []string{"80:80", "443:443"},
		}
		args := buildNerdctlRunArgs(task)
		expected := []string{"run", "-d", "--name", "myapp-web-0", "-p", "80:80", "-p", "443:443", "nginx:latest"}
		assertSliceEqual(t, expected, args)
	})

	t.Run("includes environment variables", func(t *testing.T) {
		task := &TaskRecord{
			ContainerName: "myapp-api-0",
			Image:         "myapi:v1",
			Environment:   []string{"DB=postgres", "PORT=8080"},
		}
		args := buildNerdctlRunArgs(task)
		expected := []string{"run", "-d", "--name", "myapp-api-0", "-e", "DB=postgres", "-e", "PORT=8080", "myapi:v1"}
		assertSliceEqual(t, expected, args)
	})

	t.Run("includes command", func(t *testing.T) {
		task := &TaskRecord{
			ContainerName: "myapp-api-0",
			Image:         "myapi:v1",
			Command:       []string{"serve", "--port", "8080"},
		}
		args := buildNerdctlRunArgs(task)
		expected := []string{"run", "-d", "--name", "myapp-api-0", "myapi:v1", "serve", "--port", "8080"}
		assertSliceEqual(t, expected, args)
	})

	t.Run("full task with all fields", func(t *testing.T) {
		task := &TaskRecord{
			ContainerName: "myapp-web-0",
			Image:         "nginx:latest",
			Ports:         []string{"80:80"},
			Environment:   []string{"FOO=bar"},
			Command:       []string{"nginx", "-g", "daemon off;"},
		}
		args := buildNerdctlRunArgs(task)
		expected := []string{
			"run", "-d", "--name", "myapp-web-0",
			"-p", "80:80",
			"-e", "FOO=bar",
			"nginx:latest",
			"nginx", "-g", "daemon off;",
		}
		assertSliceEqual(t, expected, args)
	})
}

func assertSliceEqual(t *testing.T, expected, got []string) {
	t.Helper()
	if len(expected) != len(got) {
		t.Errorf("length mismatch: expected %d, got %d\n  expected: %v\n  got:      %v", len(expected), len(got), expected, got)
		return
	}
	for i := range expected {
		if expected[i] != got[i] {
			t.Errorf("mismatch at index %d: expected %q, got %q\n  expected: %v\n  got:      %v", i, expected[i], got[i], expected, got)
			return
		}
	}
}
