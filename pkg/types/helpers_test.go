package types

import (
	"context"
	"encoding/json"
	"fmt"
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

	t.Run("maps restart entrypoint and resources", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"api": {
				Image:      "my-api:v1",
				Restart:    "unless-stopped",
				Entrypoint: ShellCommand{"python", "-m", "api"},
				Deploy: &ManifestDeploy{
					Replicas: 1,
					Resources: &ManifestResources{
						Limits:       &ResourceSpec{Memory: "512m", CPUs: "0.5"},
						Reservations: &ResourceSpec{Memory: "256m"},
					},
				},
			},
		}
		services := BuildServiceRecords(manifest)
		svc := services["api"]

		if svc.Restart != "unless-stopped" {
			t.Errorf("expected restart 'unless-stopped', got %q", svc.Restart)
		}
		if len(svc.Entrypoint) != 3 || svc.Entrypoint[0] != "python" {
			t.Errorf("unexpected entrypoint: %v", svc.Entrypoint)
		}
		if svc.MemoryLimit != "512m" {
			t.Errorf("expected memory limit '512m', got %q", svc.MemoryLimit)
		}
		if svc.CPULimit != "0.5" {
			t.Errorf("expected cpu limit '0.5', got %q", svc.CPULimit)
		}
		if svc.MemoryReservation != "256m" {
			t.Errorf("expected memory reservation '256m', got %q", svc.MemoryReservation)
		}
	})

	t.Run("maps healthcheck", func(t *testing.T) {
		manifest := map[string]ManifestService{
			"db": {
				Image: "postgres:15",
				Healthcheck: &ManifestHealthcheck{
					Test:        ShellCommand{"CMD", "pg_isready"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     3,
					StartPeriod: "30s",
				},
			},
		}
		services := BuildServiceRecords(manifest)
		svc := services["db"]
		if svc.Healthcheck == nil {
			t.Fatal("expected healthcheck to be set")
		}
		if len(svc.Healthcheck.Test) != 2 || svc.Healthcheck.Test[0] != "CMD" {
			t.Errorf("unexpected healthcheck test: %v", svc.Healthcheck.Test)
		}
		if svc.Healthcheck.Interval != "10s" {
			t.Errorf("expected interval '10s', got %q", svc.Healthcheck.Interval)
		}
		if svc.Healthcheck.Retries != 3 {
			t.Errorf("expected retries 3, got %d", svc.Healthcheck.Retries)
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
		tasks, _ := BuildTasksForDeployment(deployment, agents)
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
		tasks, _ := BuildTasksForDeployment(deployment, agents)

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
		tasks, _ := BuildTasksForDeployment(deployment, agents)
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
		tasks, _ := BuildTasksForDeployment(deployment, agents)
		if len(tasks) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(tasks))
		}
	})

	t.Run("uses deployment ID as container prefix when ReplacesID is set", func(t *testing.T) {
		deployment := &DeploymentRecord{
			ID:         "myapp-1234567890",
			Name:       "myapp",
			ReplacesID: "myapp-old",
			Services: map[string]ServiceRecord{
				"web": {Image: "nginx:latest", Replicas: 1},
			},
		}
		tasks, _ := BuildTasksForDeployment(deployment, agents)
		task := tasks[0]

		// Container name should use deployment ID as prefix (for blue-green uniqueness)
		expected := "myapp-1234567890-web-0"
		if task.ContainerName != expected {
			t.Errorf("expected container_name %q, got %q", expected, task.ContainerName)
		}
	})

	t.Run("uses deployment Name as container prefix when no ReplacesID", func(t *testing.T) {
		deployment := &DeploymentRecord{
			ID:   "myapp-1234567890",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"web": {Image: "nginx:latest", Replicas: 1},
			},
		}
		tasks, _ := BuildTasksForDeployment(deployment, agents)
		task := tasks[0]

		expected := "myapp-web-0"
		if task.ContainerName != expected {
			t.Errorf("expected container_name %q, got %q", expected, task.ContainerName)
		}
	})

	t.Run("placement pins service to matching agents", func(t *testing.T) {
		allAgents := []NodeRecord{
			{Name: "gateway-1"},
			{Name: "gateway-2"},
			{Name: "worker-1"},
			{Name: "worker-2"},
		}
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"proxy": {Image: "caddy", Replicas: 2, Placement: "gateway-*"},
				"api":   {Image: "myapi", Replicas: 2},
			},
		}
		tasks, err := BuildTasksForDeployment(deployment, allAgents)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, task := range tasks {
			if task.ServiceName == "proxy" {
				if task.AgentID != "gateway-1" && task.AgentID != "gateway-2" {
					t.Errorf("proxy task scheduled on %s, expected gateway-*", task.AgentID)
				}
			}
		}
	})

	t.Run("placement exact match", func(t *testing.T) {
		allAgents := []NodeRecord{
			{Name: "gateway-1"},
			{Name: "worker-1"},
		}
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"db": {Image: "postgres", Replicas: 1, Placement: "worker-1"},
			},
		}
		tasks, err := BuildTasksForDeployment(deployment, allAgents)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].AgentID != "worker-1" {
			t.Errorf("expected agent worker-1, got %s", tasks[0].AgentID)
		}
	})

	t.Run("placement error when no agents match", func(t *testing.T) {
		allAgents := []NodeRecord{
			{Name: "worker-1"},
			{Name: "worker-2"},
		}
		deployment := &DeploymentRecord{
			ID:   "deploy-1",
			Name: "myapp",
			Services: map[string]ServiceRecord{
				"proxy": {Image: "caddy", Replicas: 1, Placement: "gateway-*"},
			},
		}
		_, err := BuildTasksForDeployment(deployment, allAgents)
		if err == nil {
			t.Fatal("expected error when no agents match placement, got nil")
		}
	})

	t.Run("passes healthcheck to tasks", func(t *testing.T) {
		hc := &ManifestHealthcheck{
			Test:     ShellCommand{"CMD", "pg_isready"},
			Interval: "10s",
			Retries:  3,
		}
		deployment := &DeploymentRecord{
			ID:   "deploy-hc",
			Name: "app",
			Services: map[string]ServiceRecord{
				"db": {Image: "postgres:15", Replicas: 1, Healthcheck: hc},
			},
		}
		tasks, err := BuildTasksForDeployment(deployment, agents)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].Healthcheck == nil {
			t.Fatal("expected task to have healthcheck")
		}
		if tasks[0].Healthcheck.Interval != "10s" {
			t.Errorf("expected interval '10s', got %q", tasks[0].Healthcheck.Interval)
		}
	})
}

func TestFilterAgentsByPlacement(t *testing.T) {
	agents := []NodeRecord{
		{Name: "gateway-1"},
		{Name: "gateway-2"},
		{Name: "worker-1"},
		{Name: "worker-us-east"},
		{Name: "db-primary"},
	}

	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{"glob wildcard", "gateway-*", []string{"gateway-1", "gateway-2"}},
		{"exact match", "worker-1", []string{"worker-1"}},
		{"single char wildcard", "gateway-?", []string{"gateway-1", "gateway-2"}},
		{"prefix wildcard", "worker-*", []string{"worker-1", "worker-us-east"}},
		{"no match", "staging-*", nil},
		{"match all", "*", []string{"gateway-1", "gateway-2", "worker-1", "worker-us-east", "db-primary"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterAgentsByPlacement(agents, tt.pattern)
			if len(result) != len(tt.expected) {
				t.Errorf("pattern %q: expected %d agents, got %d", tt.pattern, len(tt.expected), len(result))
				return
			}
			for i, agent := range result {
				if agent.Name != tt.expected[i] {
					t.Errorf("pattern %q: expected agent %q at index %d, got %q", tt.pattern, tt.expected[i], i, agent.Name)
				}
			}
		})
	}
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

// helperStateStore is an in-memory StateStore for testing helpers.
type helperStateStore struct {
	data map[string]any
}

func (m *helperStateStore) Save(_ context.Context, key string, value any) error {
	m.data[key] = value
	return nil
}

func (m *helperStateStore) Get(_ context.Context, key string, dest any) error {
	v, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func (m *helperStateStore) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func TestValidateServiceDependencies(t *testing.T) {
	allServices := map[string]ServiceRecord{
		"web": {Image: "nginx", DependsOn: []string{"api", "db"}},
		"api": {Image: "myapi", DependsOn: []string{"db"}},
		"db":  {Image: "postgres"},
	}

	t.Run("deps satisfied by running services", func(t *testing.T) {
		err := ValidateServiceDependencies([]string{"web"}, allServices, []string{"api", "db"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("deps satisfied by target set", func(t *testing.T) {
		err := ValidateServiceDependencies([]string{"web", "api", "db"}, allServices, nil)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("deps satisfied by mix of running and target", func(t *testing.T) {
		err := ValidateServiceDependencies([]string{"web", "api"}, allServices, []string{"db"})
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("unsatisfied dependency returns error", func(t *testing.T) {
		err := ValidateServiceDependencies([]string{"web"}, allServices, []string{"api"})
		if err == nil {
			t.Fatal("expected error for unsatisfied dependency")
		}
		expected := `service "web" depends on "db" which is not running and not being deployed`
		if err.Error() != expected {
			t.Errorf("expected error %q, got %q", expected, err.Error())
		}
	})

	t.Run("service with no deps always passes", func(t *testing.T) {
		err := ValidateServiceDependencies([]string{"db"}, allServices, nil)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("empty target list passes", func(t *testing.T) {
		err := ValidateServiceDependencies(nil, allServices, nil)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}

func TestCollectDeploymentTasks(t *testing.T) {
	t.Run("collects tasks across multiple agents", func(t *testing.T) {
		store := &helperStateStore{data: map[string]any{}}
		ctx := context.Background()

		// Create two nodes
		store.Save(ctx, KeyNodes+"agent-1", NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, KeyNodes+"agent-2", NodeRecord{Name: "agent-2", Status: "ready"})

		// Create tasks for deployment "deploy-1"
		store.Save(ctx, KeyTasks+"agent-1/task-1", TaskRecord{ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1"})
		store.Save(ctx, KeyTasks+"agent-2/task-2", TaskRecord{ID: "task-2", DeploymentID: "deploy-1", AgentID: "agent-2"})

		// Create a task for a different deployment
		store.Save(ctx, KeyTasks+"agent-1/task-3", TaskRecord{ID: "task-3", DeploymentID: "deploy-2", AgentID: "agent-1"})

		tasks := CollectDeploymentTasks(ctx, store, "deploy-1")
		if len(tasks) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(tasks))
		}
	})

	t.Run("empty store returns nil", func(t *testing.T) {
		store := &helperStateStore{data: map[string]any{}}
		tasks := CollectDeploymentTasks(context.Background(), store, "deploy-1")
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("no matching deployment returns nil", func(t *testing.T) {
		store := &helperStateStore{data: map[string]any{}}
		ctx := context.Background()

		store.Save(ctx, KeyNodes+"agent-1", NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, KeyTasks+"agent-1/task-1", TaskRecord{ID: "task-1", DeploymentID: "other-deploy", AgentID: "agent-1"})

		tasks := CollectDeploymentTasks(ctx, store, "deploy-1")
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})
}

// errorStateStore is a StateStore that can inject errors for specific operations.
type errorStateStore struct {
	listError    error
	data         map[string]any
	getErrorKeys map[string]bool
	listErrorFor string
}

func (m *errorStateStore) Save(_ context.Context, key string, value any) error {
	m.data[key] = value
	return nil
}

func (m *errorStateStore) Get(_ context.Context, key string, dest any) error {
	if m.getErrorKeys != nil && m.getErrorKeys[key] {
		return fmt.Errorf("injected get error for key: %s", key)
	}
	v, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func (m *errorStateStore) List(_ context.Context, prefix string) ([]string, error) {
	if m.listError != nil {
		if m.listErrorFor == "" || m.listErrorFor == prefix {
			return nil, m.listError
		}
	}
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func TestCollectDeploymentTasks_NodeListError(t *testing.T) {
	store := &errorStateStore{
		data:      map[string]any{},
		listError: fmt.Errorf("store unavailable"),
	}
	tasks := CollectDeploymentTasks(context.Background(), store, "deploy-1")
	if tasks != nil {
		t.Errorf("expected nil when node list fails, got %v", tasks)
	}
}

func TestCollectDeploymentTasks_NodeGetError(t *testing.T) {
	store := &errorStateStore{
		data:         map[string]any{},
		getErrorKeys: map[string]bool{KeyNodes + "agent-bad": true},
	}
	ctx := context.Background()

	// Save a valid node and a "bad" node key
	store.Save(ctx, KeyNodes+"agent-good", NodeRecord{Name: "agent-good", Status: "ready"})
	store.data[KeyNodes+"agent-bad"] = "invalid-data" // raw string that will fail to unmarshal into NodeRecord

	// Save a task under agent-good
	store.Save(ctx, KeyTasks+"agent-good/task-1", TaskRecord{ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-good"})

	tasks := CollectDeploymentTasks(ctx, store, "deploy-1")
	// Should still collect tasks from agent-good, skipping agent-bad
	if len(tasks) != 1 {
		t.Errorf("expected 1 task (from good agent), got %d", len(tasks))
	}
}

func TestCollectDeploymentTasks_TaskListError(t *testing.T) {
	store := &errorStateStore{
		data:         map[string]any{},
		listError:    fmt.Errorf("task list error"),
		listErrorFor: KeyTasks + "agent-1/",
	}
	ctx := context.Background()

	store.Save(ctx, KeyNodes+"agent-1", NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, KeyTasks+"agent-1/task-1", TaskRecord{ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1"})

	tasks := CollectDeploymentTasks(ctx, store, "deploy-1")
	// task list fails for agent-1, so no tasks collected
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks when task list fails, got %d", len(tasks))
	}
}

func TestCollectDeploymentTasks_TaskGetError(t *testing.T) {
	store := &errorStateStore{
		data:         map[string]any{},
		getErrorKeys: map[string]bool{KeyTasks + "agent-1/task-bad": true},
	}
	ctx := context.Background()

	store.Save(ctx, KeyNodes+"agent-1", NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, KeyTasks+"agent-1/task-good", TaskRecord{ID: "task-good", DeploymentID: "deploy-1", AgentID: "agent-1"})
	store.data[KeyTasks+"agent-1/task-bad"] = "corrupt" // will cause Get error

	tasks := CollectDeploymentTasks(ctx, store, "deploy-1")
	// Should collect task-good but skip task-bad
	if len(tasks) != 1 {
		t.Errorf("expected 1 task (skipping bad task), got %d", len(tasks))
	}
}

func TestTagsMatch(t *testing.T) {
	tests := []struct {
		name           string
		agentTags      []string
		deploymentTags []string
		want           bool
	}{
		{
			name:           "both empty matches",
			agentTags:      nil,
			deploymentTags: nil,
			want:           true,
		},
		{
			name:           "both empty slices matches",
			agentTags:      []string{},
			deploymentTags: []string{},
			want:           true,
		},
		{
			name:           "agent empty deployment non-empty does not match",
			agentTags:      nil,
			deploymentTags: []string{"gpu"},
			want:           false,
		},
		{
			name:           "agent non-empty deployment empty does not match",
			agentTags:      []string{"gpu"},
			deploymentTags: nil,
			want:           false,
		},
		{
			name:           "both non-empty with intersection matches",
			agentTags:      []string{"gpu", "ssd"},
			deploymentTags: []string{"ssd", "large"},
			want:           true,
		},
		{
			name:           "both non-empty without intersection does not match",
			agentTags:      []string{"gpu", "ssd"},
			deploymentTags: []string{"arm", "large"},
			want:           false,
		},
		{
			name:           "exact same tags matches",
			agentTags:      []string{"gpu"},
			deploymentTags: []string{"gpu"},
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TagsMatch(tt.agentTags, tt.deploymentTags)
			if got != tt.want {
				t.Errorf("TagsMatch(%v, %v) = %v, want %v", tt.agentTags, tt.deploymentTags, got, tt.want)
			}
		})
	}
}

func TestTagsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "both empty",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "same set different order",
			a:    []string{"gpu", "ssd", "arm"},
			b:    []string{"arm", "gpu", "ssd"},
			want: true,
		},
		{
			name: "different sets same length",
			a:    []string{"gpu", "ssd"},
			b:    []string{"gpu", "arm"},
			want: false,
		},
		{
			name: "different lengths",
			a:    []string{"gpu"},
			b:    []string{"gpu", "ssd"},
			want: false,
		},
		{
			name: "identical single element",
			a:    []string{"gpu"},
			b:    []string{"gpu"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TagsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("TagsEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSortTags(t *testing.T) {
	t.Run("returns sorted copy", func(t *testing.T) {
		original := []string{"cherry", "apple", "banana"}
		sorted := SortTags(original)

		expected := []string{"apple", "banana", "cherry"}
		assertSliceEqual(t, expected, sorted)
	})

	t.Run("does not mutate original", func(t *testing.T) {
		original := []string{"cherry", "apple", "banana"}
		_ = SortTags(original)

		// original must remain unchanged
		expectedOriginal := []string{"cherry", "apple", "banana"}
		assertSliceEqual(t, expectedOriginal, original)
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		sorted := SortTags(nil)
		if sorted != nil {
			t.Errorf("expected nil for empty input, got %v", sorted)
		}
	})

	t.Run("returns nil for zero-length slice", func(t *testing.T) {
		sorted := SortTags([]string{})
		if sorted != nil {
			t.Errorf("expected nil for zero-length slice, got %v", sorted)
		}
	})

	t.Run("single element", func(t *testing.T) {
		sorted := SortTags([]string{"only"})
		assertSliceEqual(t, []string{"only"}, sorted)
	})

	t.Run("already sorted input", func(t *testing.T) {
		sorted := SortTags([]string{"a", "b", "c"})
		assertSliceEqual(t, []string{"a", "b", "c"}, sorted)
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
