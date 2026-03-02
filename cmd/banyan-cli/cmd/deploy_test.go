package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestValidateManifest(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Name: "my-app",
			Services: map[string]types.ManifestService{
				"web": {Image: "nginx:alpine"},
			},
		}
		if err := validateManifest(manifest); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("valid with build config", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Name: "my-app",
			Services: map[string]types.ManifestService{
				"web": {Build: &types.ManifestBuild{Context: "./web"}},
			},
		}
		if err := validateManifest(manifest); err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Services: map[string]types.ManifestService{
				"web": {Image: "nginx"},
			},
		}
		if err := validateManifest(manifest); err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("no services", func(t *testing.T) {
		manifest := types.BanyanManifest{Name: "my-app"}
		if err := validateManifest(manifest); err == nil {
			t.Error("expected error for no services")
		}
	})

	t.Run("service without image or build", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Name: "my-app",
			Services: map[string]types.ManifestService{
				"web": {},
			},
		}
		if err := validateManifest(manifest); err == nil {
			t.Error("expected error for service without image or build")
		}
	})
}

func TestValidateServiceArgs(t *testing.T) {
	services := map[string]types.ManifestService{
		"web": {Image: "nginx"},
		"api": {Image: "node"},
		"db":  {Image: "postgres"},
	}

	t.Run("valid service names", func(t *testing.T) {
		err := validateServiceArgs([]string{"web", "api"}, services)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("unknown service name", func(t *testing.T) {
		err := validateServiceArgs([]string{"web", "redis"}, services)
		if err == nil {
			t.Fatal("expected error for unknown service")
		}
		expected := `service "redis" not found in manifest`
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("empty args passes", func(t *testing.T) {
		err := validateServiceArgs(nil, services)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})
}

func TestBuildImageArgs(t *testing.T) {
	t.Run("without dockerfile", func(t *testing.T) {
		args := buildImageArgs("my-app:latest", "./web", "")
		expected := []string{"build", "-t", "my-app:latest", "./web"}
		if len(args) != len(expected) {
			t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
		}
		for i, exp := range expected {
			if args[i] != exp {
				t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
			}
		}
	})

	t.Run("with dockerfile", func(t *testing.T) {
		args := buildImageArgs("my-app:latest", "./web", "Dockerfile.prod")
		expectedPath := filepath.Join("./web", "Dockerfile.prod")
		expected := []string{"build", "-t", "my-app:latest", "-f", expectedPath, "./web"}
		if len(args) != len(expected) {
			t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
		}
		for i, exp := range expected {
			if args[i] != exp {
				t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
			}
		}
	})
}

func TestBuildServiceImages_NoBuildNeeded(t *testing.T) {
	services := map[string]types.ManifestService{
		"web": {Image: "nginx:alpine"},
		"api": {Image: "node:18"},
	}
	err := buildServiceImages("/tmp", "myapp", services)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestPushServiceImages(t *testing.T) {
	t.Run("no build services skips push", func(t *testing.T) {
		services := map[string]types.ManifestService{
			"web": {Image: "nginx:alpine"},
		}
		err := pushServiceImages("localhost:5000", services)
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("empty registry URL prints warning", func(t *testing.T) {
		services := map[string]types.ManifestService{
			"web": {Image: "nginx:alpine", Build: &types.ManifestBuild{Context: "./web"}},
		}
		err := pushServiceImages("", services)
		if err != nil {
			t.Errorf("expected nil error (warning only), got %v", err)
		}
	})
}

func TestResolveAppName(t *testing.T) {
	t.Run("explicit name", func(t *testing.T) {
		name, err := resolveAppName("my-app", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "my-app" {
			t.Errorf("expected 'my-app', got %q", name)
		}
	})

	t.Run("name takes precedence over file", func(t *testing.T) {
		name, err := resolveAppName("my-app", "some-file.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "my-app" {
			t.Errorf("expected 'my-app', got %q", name)
		}
	})

	t.Run("from manifest file", func(t *testing.T) {
		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "banyan.yaml")
		os.WriteFile(manifestPath, []byte("name: file-app\nservices:\n  web:\n    image: nginx\n"), 0o644)

		name, err := resolveAppName("", manifestPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "file-app" {
			t.Errorf("expected 'file-app', got %q", name)
		}
	})

	t.Run("no name or file", func(t *testing.T) {
		_, err := resolveAppName("", "")
		if err == nil {
			t.Error("expected error when both name and file are empty")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := resolveAppName("", "/nonexistent/file.yaml")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "bad.yaml")
		os.WriteFile(manifestPath, []byte("name: [unterminated"), 0o644)

		_, err := resolveAppName("", manifestPath)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("manifest without name", func(t *testing.T) {
		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "noname.yaml")
		os.WriteFile(manifestPath, []byte("services:\n  web:\n    image: nginx\n"), 0o644)

		_, err := resolveAppName("", manifestPath)
		if err == nil {
			t.Error("expected error for manifest without name")
		}
	})
}

func TestBuildServiceImages_WithBuild(t *testing.T) {
	origBuildImageFunc := buildImageFunc
	t.Cleanup(func() { buildImageFunc = origBuildImageFunc })

	var calls []string
	buildImageFunc = func(imageName, contextPath, dockerfile string) error {
		calls = append(calls, imageName)
		return nil
	}

	services := map[string]types.ManifestService{
		"web": {
			Image: "myapp-web:latest",
			Build: &types.ManifestBuild{Context: "./web"},
		},
		"api": {
			Image: "myapp-api:latest",
			Build: &types.ManifestBuild{Context: "./api", Dockerfile: "Dockerfile.prod"},
		},
	}

	err := buildServiceImages("/tmp", "myapp", services)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 build calls, got %d", len(calls))
	}
}

func TestBuildServiceImages_BuildFails(t *testing.T) {
	origBuildImageFunc := buildImageFunc
	t.Cleanup(func() { buildImageFunc = origBuildImageFunc })

	buildImageFunc = func(imageName, contextPath, dockerfile string) error {
		return fmt.Errorf("build failed")
	}

	services := map[string]types.ManifestService{
		"web": {
			Image: "myapp-web:latest",
			Build: &types.ManifestBuild{Context: "./web"},
		},
	}

	err := buildServiceImages("/tmp", "myapp", services)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `failed to build image for service "web": build failed` {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestBuildServiceImages_SetsDefaultImage(t *testing.T) {
	origBuildImageFunc := buildImageFunc
	t.Cleanup(func() { buildImageFunc = origBuildImageFunc })

	buildImageFunc = func(imageName, contextPath, dockerfile string) error {
		return nil
	}

	services := map[string]types.ManifestService{
		"web": {
			Build: &types.ManifestBuild{Context: "./web"},
		},
	}

	err := buildServiceImages("/tmp", "myapp", services)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if services["web"].Image != "myapp-web:latest" {
		t.Errorf("expected default image 'myapp-web:latest', got %q", services["web"].Image)
	}
}

func TestPushServiceImages_WithBuild(t *testing.T) {
	origTagImageFunc := tagImageFunc
	origPushImageFunc := pushImageFunc
	t.Cleanup(func() {
		tagImageFunc = origTagImageFunc
		pushImageFunc = origPushImageFunc
	})

	var tagCalls [][2]string
	var pushCalls []string
	tagImageFunc = func(src, dst string) error {
		tagCalls = append(tagCalls, [2]string{src, dst})
		return nil
	}
	pushImageFunc = func(image string) error {
		pushCalls = append(pushCalls, image)
		return nil
	}

	services := map[string]types.ManifestService{
		"web": {
			Image: "myapp-web:latest",
			Build: &types.ManifestBuild{Context: "./web"},
		},
	}

	err := pushServiceImages("localhost:5000", services)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(tagCalls) != 1 {
		t.Fatalf("expected 1 tag call, got %d", len(tagCalls))
	}
	if tagCalls[0][0] != "myapp-web:latest" {
		t.Errorf("expected tag src 'myapp-web:latest', got %q", tagCalls[0][0])
	}
	if tagCalls[0][1] != "localhost:5000/myapp-web:latest" {
		t.Errorf("expected tag dst 'localhost:5000/myapp-web:latest', got %q", tagCalls[0][1])
	}

	if len(pushCalls) != 1 {
		t.Fatalf("expected 1 push call, got %d", len(pushCalls))
	}
	if pushCalls[0] != "localhost:5000/myapp-web:latest" {
		t.Errorf("expected push image 'localhost:5000/myapp-web:latest', got %q", pushCalls[0])
	}

	// Verify service image was updated to registry-prefixed name
	if services["web"].Image != "localhost:5000/myapp-web:latest" {
		t.Errorf("expected service image updated to 'localhost:5000/myapp-web:latest', got %q", services["web"].Image)
	}
}

func TestPushServiceImages_TagFails(t *testing.T) {
	origTagImageFunc := tagImageFunc
	origPushImageFunc := pushImageFunc
	t.Cleanup(func() {
		tagImageFunc = origTagImageFunc
		pushImageFunc = origPushImageFunc
	})

	tagImageFunc = func(src, dst string) error {
		return fmt.Errorf("tag failed")
	}
	pushImageFunc = func(image string) error {
		t.Error("push should not be called when tag fails")
		return nil
	}

	services := map[string]types.ManifestService{
		"web": {
			Image: "myapp-web:latest",
			Build: &types.ManifestBuild{Context: "./web"},
		},
	}

	err := pushServiceImages("localhost:5000", services)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `failed to tag image for service "web": tag failed` {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestPushServiceImages_PushFails(t *testing.T) {
	origTagImageFunc := tagImageFunc
	origPushImageFunc := pushImageFunc
	t.Cleanup(func() {
		tagImageFunc = origTagImageFunc
		pushImageFunc = origPushImageFunc
	})

	tagImageFunc = func(src, dst string) error {
		return nil
	}
	pushImageFunc = func(image string) error {
		return fmt.Errorf("push failed")
	}

	services := map[string]types.ManifestService{
		"web": {
			Image: "myapp-web:latest",
			Build: &types.ManifestBuild{Context: "./web"},
		},
	}

	err := pushServiceImages("localhost:5000", services)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `failed to push image for service "web": push failed` {
		t.Errorf("unexpected error message: %s", got)
	}
}

// writeTestManifest creates a temp manifest YAML and returns its path.
func writeTestManifest(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "banyan.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
	return path
}

// saveDeployFlags saves current deploy flag values and restores them on cleanup.
func saveDeployFlags(t *testing.T) {
	t.Helper()
	origFile := deployFile
	origDryRun := deployDryRun
	origNoWait := deployNoWait
	origTags := deployTags
	t.Cleanup(func() {
		deployFile = origFile
		deployDryRun = origDryRun
		deployNoWait = origNoWait
		deployTags = origTags
	})
}

func TestRunDeploy_DryRun(t *testing.T) {
	saveDeployFlags(t)

	manifest := `name: my-app
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
  api:
    image: node:18
    deploy:
      replicas: 2
`
	deployFile = writeTestManifest(t, manifest)
	deployDryRun = true
	deployNoWait = false
	deployTags = nil

	err := runDeploy(deployCmd, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunDeploy_DryRun_WithEnvFile(t *testing.T) {
	saveDeployFlags(t)

	tmpDir := t.TempDir()

	// Write .env file
	envContent := "DB_HOST=localhost\nDB_PORT=5432\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0o644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Write manifest with env_file
	manifest := `name: my-app
services:
  web:
    image: nginx:alpine
    env_file: .env
    environment:
      - APP_ENV=production
`
	manifestPath := filepath.Join(tmpDir, "banyan.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	deployFile = manifestPath
	deployDryRun = true
	deployNoWait = false
	deployTags = nil

	err := runDeploy(deployCmd, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunDeploy_DryRun_MissingEnvFile(t *testing.T) {
	saveDeployFlags(t)

	tmpDir := t.TempDir()
	manifest := `name: my-app
services:
  web:
    image: nginx:alpine
    env_file: .env.nonexistent
`
	manifestPath := filepath.Join(tmpDir, "banyan.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	deployFile = manifestPath
	deployDryRun = true

	err := runDeploy(deployCmd, nil)
	if err == nil {
		t.Fatal("expected error for missing env file")
	}
	if !strings.Contains(err.Error(), "failed to resolve env_file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeploy_DryRun_InvalidManifest(t *testing.T) {
	saveDeployFlags(t)

	deployFile = writeTestManifest(t, "services:\n  web:\n    image: nginx\n")
	deployDryRun = true

	err := runDeploy(deployCmd, nil)
	if err == nil {
		t.Fatal("expected error for manifest without name")
	}
}

func TestRunDeploy_NonexistentFile(t *testing.T) {
	saveDeployFlags(t)

	deployFile = "/nonexistent/banyan.yaml"
	deployDryRun = true

	err := runDeploy(deployCmd, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to read manifest") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeploy_InvalidServiceArgs(t *testing.T) {
	saveDeployFlags(t)

	deployFile = writeTestManifest(t, "name: my-app\nservices:\n  web:\n    image: nginx\n")
	deployDryRun = true

	err := runDeploy(deployCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown service name")
	}
	if !strings.Contains(err.Error(), "not found in manifest") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeploy_NoWait(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()
	setupCLITestConfig(t, addr)
	saveDeployFlags(t)

	deployFile = writeTestManifest(t, "name: my-app\nservices:\n  web:\n    image: nginx:alpine\n")
	deployDryRun = false
	deployNoWait = true
	deployTags = nil

	err := runDeploy(deployCmd, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunDeploy_WithWait(t *testing.T) {
	// The default cliTestServer returns StatusRunning for "my-app",
	// so waitForDeployment should find it and return immediately.
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()
	setupCLITestConfig(t, addr)
	saveDeployFlags(t)

	deployFile = writeTestManifest(t, "name: my-app\nservices:\n  web:\n    image: nginx:alpine\n")
	deployDryRun = false
	deployNoWait = false
	deployTags = nil

	err := runDeploy(deployCmd, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// setupBufconnClientWithStatus creates a bufconn gRPC client backed by a server
// that returns the given deployment status for app "my-app".
func setupBufconnClientWithStatus(t *testing.T, status string) *EngineClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &deploymentStatusServer{status: status})
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	t.Cleanup(func() { srv.Stop() })

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}
}

// deploymentStatusServer returns a configurable deployment status.
type deploymentStatusServer struct {
	banyanpb.UnimplementedEngineServiceServer
	status string
}

func (s *deploymentStatusServer) GetStatus(_ context.Context, _ *banyanpb.GetStatusRequest) (*banyanpb.GetStatusResponse, error) {
	return &banyanpb.GetStatusResponse{
		Deployments: []*banyanpb.DeploymentInfo{
			{Name: "my-app", Status: s.status, CreatedAtUnix: time.Now().Unix()},
		},
	}, nil
}

func TestWaitForDeployment_Running(t *testing.T) {
	origTags := deployTags
	t.Cleanup(func() { deployTags = origTags })
	deployTags = nil

	client := setupBufconnClientWithStatus(t, types.StatusRunning)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := waitForDeployment(ctx, client, "my-app")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWaitForDeployment_Failed(t *testing.T) {
	origTags := deployTags
	t.Cleanup(func() { deployTags = origTags })
	deployTags = nil

	client := setupBufconnClientWithStatus(t, types.StatusFailed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := waitForDeployment(ctx, client, "my-app")
	if err == nil {
		t.Fatal("expected error for failed deployment")
	}
	if !strings.Contains(err.Error(), "deployment failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForDeployment_Timeout(t *testing.T) {
	origTags := deployTags
	t.Cleanup(func() { deployTags = origTags })
	deployTags = nil

	// Use StatusPending so it never resolves
	client := setupBufconnClientWithStatus(t, types.StatusPending)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := waitForDeployment(ctx, client, "my-app")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForDeployment_Deploying(t *testing.T) {
	origTags := deployTags
	t.Cleanup(func() { deployTags = origTags })
	deployTags = nil

	// StatusDeploying should be logged but not returned — will eventually timeout
	client := setupBufconnClientWithStatus(t, types.StatusDeploying)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := waitForDeployment(ctx, client, "my-app")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRunDeploy_InvalidYAML(t *testing.T) {
	saveDeployFlags(t)

	deployFile = writeTestManifest(t, ": [invalid yaml")
	deployDryRun = true

	err := runDeploy(deployCmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "failed to parse manifest") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeploy_NoEngineConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })
	saveDeployFlags(t)

	configPath = "/tmp/nonexistent-deploy-config.yaml"
	deployFile = writeTestManifest(t, "name: my-app\nservices:\n  web:\n    image: nginx\n")
	deployDryRun = false
	deployNoWait = false

	err := runDeploy(deployCmd, nil)
	if err == nil {
		t.Fatal("expected error when no engine config")
	}
	if !strings.Contains(err.Error(), "engine endpoint not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeploy_ClientConnectFails(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })
	saveDeployFlags(t)

	// Config with engine endpoint but no EngineWGPublicKey
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:  "127.0.0.1",
			EnginePort:  "50051",
			WGPublicKey: "test-pubkey",
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	deployFile = writeTestManifest(t, "name: my-app\nservices:\n  web:\n    image: nginx\n")
	deployDryRun = false
	deployNoWait = false

	err := runDeploy(deployCmd, nil)
	if err == nil {
		t.Fatal("expected error when client connect fails")
	}
	if !strings.Contains(err.Error(), "failed to connect to engine") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeploy_WithServiceArgs(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()
	setupCLITestConfig(t, addr)
	saveDeployFlags(t)

	deployFile = writeTestManifest(t, "name: my-app\nservices:\n  web:\n    image: nginx:alpine\n  api:\n    image: node:18\n")
	deployDryRun = false
	deployNoWait = true
	deployTags = nil

	err := runDeploy(deployCmd, []string{"web"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
