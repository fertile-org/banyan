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

func TestManifestPlacementParsing(t *testing.T) {
	t.Run("parses placement node", func(t *testing.T) {
		input := `
name: my-app
services:
  proxy:
    image: caddy:latest
    deploy:
      placement:
        node: gateway-*
      replicas: 2
  api:
    image: myapi
    deploy:
      replicas: 3
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		proxy := manifest.Services["proxy"]
		if proxy.Deploy == nil || proxy.Deploy.Placement == nil {
			t.Fatal("expected proxy to have deploy.placement")
		}
		if proxy.Deploy.Placement.Node != "gateway-*" {
			t.Errorf("expected placement node 'gateway-*', got %q", proxy.Deploy.Placement.Node)
		}
		if proxy.Deploy.Replicas != 2 {
			t.Errorf("expected 2 replicas, got %d", proxy.Deploy.Replicas)
		}

		api := manifest.Services["api"]
		if api.Deploy != nil && api.Deploy.Placement != nil {
			t.Error("expected api to have no placement")
		}
	})

	t.Run("no placement is nil", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
    deploy:
      replicas: 1
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		web := manifest.Services["web"]
		if web.Deploy.Placement != nil {
			t.Error("expected nil placement")
		}
	})
}
