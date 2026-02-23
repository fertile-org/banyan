package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
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
