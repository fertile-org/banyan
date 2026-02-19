package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGetReplicas(t *testing.T) {
	t.Run("nil deploy returns 0", func(t *testing.T) {
		svc := ManifestService{Image: "nginx"}
		if got := svc.GetReplicas(); got != 0 {
			t.Errorf("expected 0, got %d", got)
		}
	})

	t.Run("returns configured replicas", func(t *testing.T) {
		svc := ManifestService{
			Image:  "nginx",
			Deploy: &ManifestDeploy{Replicas: 5},
		}
		if got := svc.GetReplicas(); got != 5 {
			t.Errorf("expected 5, got %d", got)
		}
	})
}

func TestManifestBuildUnmarshalYAML(t *testing.T) {
	t.Run("string form", func(t *testing.T) {
		input := `build: ./mypath`
		var svc struct {
			Build *ManifestBuild `yaml:"build"`
		}
		if err := yaml.Unmarshal([]byte(input), &svc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if svc.Build == nil {
			t.Fatal("expected non-nil Build")
		}
		if svc.Build.Context != "./mypath" {
			t.Errorf("expected context './mypath', got %q", svc.Build.Context)
		}
		if svc.Build.Dockerfile != "" {
			t.Errorf("expected empty dockerfile, got %q", svc.Build.Dockerfile)
		}
	})

	t.Run("object form", func(t *testing.T) {
		input := `build:
  context: ./app
  dockerfile: Dockerfile.prod`
		var svc struct {
			Build *ManifestBuild `yaml:"build"`
		}
		if err := yaml.Unmarshal([]byte(input), &svc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if svc.Build == nil {
			t.Fatal("expected non-nil Build")
		}
		if svc.Build.Context != "./app" {
			t.Errorf("expected context './app', got %q", svc.Build.Context)
		}
		if svc.Build.Dockerfile != "Dockerfile.prod" {
			t.Errorf("expected dockerfile 'Dockerfile.prod', got %q", svc.Build.Dockerfile)
		}
	})
}
