package types

import (
	"testing"
)

func TestBuildServiceRecords(t *testing.T) {
	t.Run("defaults replicas to 1 when zero", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"web": {Image: "nginx:latest"},
		}
		services := BuildServiceRecords(manifest)
		if services["web"].Replicas != 1 {
			t.Errorf("expected replicas=1, got %d", services["web"].Replicas)
		}
	})

	t.Run("preserves explicit replicas", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"web": {Image: "nginx:latest", Deploy: &ManifestDeploy{Replicas: 3}},
		}
		services := BuildServiceRecords(manifest)
		if services["web"].Replicas != 3 {
			t.Errorf("expected replicas=3, got %d", services["web"].Replicas)
		}
	})

	t.Run("maps all fields correctly", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"api": {
				Image:       "my-api:v1",
				Deploy:      &ManifestDeploy{Replicas: 2},
				Ports:       []string{"8080:80"},
				Environment: []string{"DB=postgres"},
				Command:     []string{"serve"},
				DependsOn:   []string{"db"},
			},
		}
		services := BuildServiceRecords(manifest)
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
		tasks := BuildTasksForDeployment(deployment, agents)
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
		tasks := BuildTasksForDeployment(deployment, agents)

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
		tasks := BuildTasksForDeployment(deployment, agents)
		task := tasks[0]

		if task.DeploymentID != "deploy-1" {
			t.Errorf("unexpected deployment_id: %s", task.DeploymentID)
		}
		if task.ServiceName != "web" {
			t.Errorf("unexpected service_name: %s", task.ServiceName)
		}
		if task.Type != TaskTypeCreateAndStart {
			t.Errorf("unexpected type: %s", task.Type)
		}
		if task.Status != StatusPending {
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
		tasks := BuildTasksForDeployment(deployment, agents)
		if len(tasks) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(tasks))
		}
	})
}

func TestDetermineDeploymentStatus(t *testing.T) {
	t.Run("returns empty when no tasks", func(t *testing.T) {
		status, _ := DetermineDeploymentStatus(0, 0, 0, "")
		if status != "" {
			t.Errorf("expected empty status, got %s", status)
		}
	})

	t.Run("returns running when all completed", func(t *testing.T) {
		status, errMsg := DetermineDeploymentStatus(3, 3, 0, "")
		if status != StatusRunning {
			t.Errorf("expected running, got %s", status)
		}
		if errMsg != "" {
			t.Errorf("expected no error, got %s", errMsg)
		}
	})

	t.Run("returns failed when any task failed", func(t *testing.T) {
		status, errMsg := DetermineDeploymentStatus(3, 1, 1, "pull failed")
		if status != StatusFailed {
			t.Errorf("expected failed, got %s", status)
		}
		if errMsg == "" {
			t.Error("expected error message")
		}
	})

	t.Run("returns empty when still in progress", func(t *testing.T) {
		status, _ := DetermineDeploymentStatus(3, 1, 0, "")
		if status != "" {
			t.Errorf("expected empty (in progress), got %s", status)
		}
	})

	t.Run("failed takes priority over completed", func(t *testing.T) {
		status, _ := DetermineDeploymentStatus(3, 2, 1, "timeout")
		if status != StatusFailed {
			t.Errorf("expected failed, got %s", status)
		}
	})
}

func TestGroupTasksByService(t *testing.T) {
	t.Run("groups tasks by service name", func(t *testing.T) {
		tasks := []TaskRecord{
			{ServiceName: "web", ContainerName: "myapp-web-0"},
			{ServiceName: "api", ContainerName: "myapp-api-0"},
			{ServiceName: "web", ContainerName: "myapp-web-1"},
			{ServiceName: "api", ContainerName: "myapp-api-1"},
			{ServiceName: "db", ContainerName: "myapp-db-0"},
		}
		grouped := GroupTasksByService(tasks)

		if len(grouped) != 3 {
			t.Errorf("expected 3 groups, got %d", len(grouped))
		}
		if len(grouped["web"]) != 2 {
			t.Errorf("expected 2 web tasks, got %d", len(grouped["web"]))
		}
		if len(grouped["api"]) != 2 {
			t.Errorf("expected 2 api tasks, got %d", len(grouped["api"]))
		}
		if len(grouped["db"]) != 1 {
			t.Errorf("expected 1 db task, got %d", len(grouped["db"]))
		}
	})

	t.Run("handles empty input", func(t *testing.T) {
		grouped := GroupTasksByService(nil)
		if len(grouped) != 0 {
			t.Errorf("expected 0 groups, got %d", len(grouped))
		}
	})
}

func TestSortedServiceNames(t *testing.T) {
	t.Run("returns sorted names", func(t *testing.T) {
		grouped := map[string][]TaskRecord{
			"web": {{ServiceName: "web"}},
			"api": {{ServiceName: "api"}},
			"db":  {{ServiceName: "db"}},
		}
		names := SortedServiceNames(grouped)
		expected := []string{"api", "db", "web"}
		assertSliceEqual(t, expected, names)
	})

	t.Run("handles empty map", func(t *testing.T) {
		grouped := map[string][]TaskRecord{}
		names := SortedServiceNames(grouped)
		if len(names) != 0 {
			t.Errorf("expected 0 names, got %d", len(names))
		}
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
			t.Errorf("mismatch at index %d: expected %q, got %q", i, expected[i], got[i])
			return
		}
	}
}
