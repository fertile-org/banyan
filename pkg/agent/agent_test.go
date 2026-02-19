package agent

import (
	"testing"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

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
