package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "web", false},
		{"valid with hyphens", "my-api-server", false},
		{"valid with numbers", "app1", false},
		{"valid single char", "a", false},
		{"valid all digits", "123", false},
		{"empty", "", true},
		{"uppercase", "MyApp", true},
		{"starts with hyphen", "-web", true},
		{"ends with hyphen", "web-", true},
		{"contains dot", "web.server", true},
		{"contains slash", "web/api", true},
		{"contains space", "web api", true},
		{"valid with underscore", "web_api", false},
		{"contains null byte", "web\x00api", true},
		{"too long", string(make([]byte, 64)), true},
		{"max length 63", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"path traversal", "../etc/passwd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{"valid 80:80", "80:80", false},
		{"valid 8080:80", "8080:80", false},
		{"valid with tcp", "80:80/tcp", false},
		{"valid with udp", "53:53/udp", false},
		{"valid high port", "65535:65535", false},
		{"valid port 1", "1:1", false},
		{"missing colon", "8080", true},
		{"port 0", "0:80", true},
		{"port 65536", "65536:80", true},
		{"negative port", "-1:80", true},
		{"non-numeric", "abc:80", true},
		{"empty host", ":80", true},
		{"empty container", "80:", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRestartPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantErr bool
	}{
		{"empty (omitted)", "", false},
		{"no", "no", false},
		{"always", "always", false},
		{"unless-stopped", "unless-stopped", false},
		{"on-failure", "on-failure", false},
		{"on-failure:3", "on-failure:3", false},
		{"on-failure:0", "on-failure:0", false},
		{"invalid", "restart-always", true},
		{"on-failure:abc", "on-failure:abc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRestartPolicy(tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRestartPolicy(%q) error = %v, wantErr %v", tt.policy, err, tt.wantErr)
			}
		})
	}
}

func TestValidateService(t *testing.T) {
	t.Run("valid service with image", func(t *testing.T) {
		err := ValidateService("web", &ManifestService{Image: "nginx:alpine"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid service with build", func(t *testing.T) {
		err := ValidateService("api", &ManifestService{Build: &ManifestBuild{Context: "."}})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		err := ValidateService("MY_APP", &ManifestService{Image: "nginx"})
		if err == nil {
			t.Error("expected error for uppercase name")
		}
	})

	t.Run("no image or build", func(t *testing.T) {
		err := ValidateService("web", &ManifestService{})
		if err == nil {
			t.Error("expected error for missing image and build")
		}
	})

	t.Run("invalid port", func(t *testing.T) {
		err := ValidateService("web", &ManifestService{
			Image: "nginx",
			Ports: []string{"99999:80"},
		})
		if err == nil {
			t.Error("expected error for invalid port")
		}
	})

	t.Run("replicas exceed max", func(t *testing.T) {
		err := ValidateService("web", &ManifestService{
			Image:  "nginx",
			Deploy: &ManifestDeploy{Replicas: MaxReplicas + 1},
		})
		if err == nil {
			t.Error("expected error for too many replicas")
		}
	})

	t.Run("invalid restart policy", func(t *testing.T) {
		err := ValidateService("web", &ManifestService{
			Image:   "nginx",
			Restart: "invalid-policy",
		})
		if err == nil {
			t.Error("expected error for invalid restart policy")
		}
	})
}

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

	t.Run("env_file string form in manifest", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
    env_file: .env
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		web := manifest.Services["web"]
		if len(web.EnvFile) != 1 || web.EnvFile[0] != ".env" {
			t.Errorf("expected ['.env'], got %v", web.EnvFile)
		}
	})

	t.Run("env_file list form in manifest", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
    env_file:
      - .env
      - .env.local
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		web := manifest.Services["web"]
		if len(web.EnvFile) != 2 {
			t.Fatalf("expected 2 env_file entries, got %d", len(web.EnvFile))
		}
		if web.EnvFile[0] != ".env" {
			t.Errorf("expected '.env', got %q", web.EnvFile[0])
		}
		if web.EnvFile[1] != ".env.local" {
			t.Errorf("expected '.env.local', got %q", web.EnvFile[1])
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

func TestManifestRestartParsing(t *testing.T) {
	input := `
name: my-app
services:
  web:
    image: nginx
    restart: unless-stopped
  db:
    image: postgres
    restart: on-failure:3
  worker:
    image: python:3
`
	var manifest BanyanManifest
	if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if manifest.Services["web"].Restart != "unless-stopped" {
		t.Errorf("expected 'unless-stopped', got %q", manifest.Services["web"].Restart)
	}
	if manifest.Services["db"].Restart != "on-failure:3" {
		t.Errorf("expected 'on-failure:3', got %q", manifest.Services["db"].Restart)
	}
	if manifest.Services["worker"].Restart != "" {
		t.Errorf("expected empty restart, got %q", manifest.Services["worker"].Restart)
	}
}

func TestManifestEntrypointParsing(t *testing.T) {
	t.Run("list form", func(t *testing.T) {
		input := `
name: my-app
services:
  db:
    image: postgres
    entrypoint:
      - docker-entrypoint.sh
      - --config
      - /etc/pg.conf
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		ep := manifest.Services["db"].Entrypoint
		if len(ep) != 3 {
			t.Fatalf("expected 3 entrypoint args, got %d: %v", len(ep), ep)
		}
		if ep[0] != "docker-entrypoint.sh" || ep[1] != "--config" || ep[2] != "/etc/pg.conf" {
			t.Errorf("unexpected entrypoint: %v", ep)
		}
	})

	t.Run("string form", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
    entrypoint: /custom-entrypoint.sh
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		ep := manifest.Services["web"].Entrypoint
		if len(ep) != 1 || ep[0] != "/custom-entrypoint.sh" {
			t.Errorf("expected [/custom-entrypoint.sh], got %v", ep)
		}
	})
}

func TestManifestResourcesParsing(t *testing.T) {
	input := `
name: my-app
services:
  api:
    image: my-api
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 512m
          cpus: "0.5"
        reservations:
          memory: 256m
          cpus: "0.25"
`
	var manifest BanyanManifest
	if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	api := manifest.Services["api"]
	if api.Deploy == nil || api.Deploy.Resources == nil {
		t.Fatal("expected deploy.resources to be set")
	}
	if api.Deploy.Resources.Limits == nil {
		t.Fatal("expected limits to be set")
	}
	if api.Deploy.Resources.Limits.Memory != "512m" {
		t.Errorf("expected memory limit '512m', got %q", api.Deploy.Resources.Limits.Memory)
	}
	if api.Deploy.Resources.Limits.CPUs != "0.5" {
		t.Errorf("expected cpu limit '0.5', got %q", api.Deploy.Resources.Limits.CPUs)
	}
	if api.Deploy.Resources.Reservations == nil {
		t.Fatal("expected reservations to be set")
	}
	if api.Deploy.Resources.Reservations.Memory != "256m" {
		t.Errorf("expected memory reservation '256m', got %q", api.Deploy.Resources.Reservations.Memory)
	}
}

func TestManifestHealthcheckParsing(t *testing.T) {
	t.Run("list form (CMD)", func(t *testing.T) {
		input := `
name: my-app
services:
  db:
    image: postgres:15
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "postgres"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		db := manifest.Services["db"]
		if db.Healthcheck == nil {
			t.Fatal("expected healthcheck to be set")
		}
		if len(db.Healthcheck.Test) != 4 {
			t.Fatalf("expected 4 test entries, got %d", len(db.Healthcheck.Test))
		}
		if db.Healthcheck.Test[0] != "CMD" {
			t.Errorf("expected test[0] 'CMD', got %q", db.Healthcheck.Test[0])
		}
		if db.Healthcheck.Test[1] != "pg_isready" {
			t.Errorf("expected test[1] 'pg_isready', got %q", db.Healthcheck.Test[1])
		}
		if db.Healthcheck.Interval != "10s" {
			t.Errorf("expected interval '10s', got %q", db.Healthcheck.Interval)
		}
		if db.Healthcheck.Timeout != "5s" {
			t.Errorf("expected timeout '5s', got %q", db.Healthcheck.Timeout)
		}
		if db.Healthcheck.Retries != 3 {
			t.Errorf("expected retries 3, got %d", db.Healthcheck.Retries)
		}
		if db.Healthcheck.StartPeriod != "30s" {
			t.Errorf("expected start_period '30s', got %q", db.Healthcheck.StartPeriod)
		}
	})

	t.Run("string form (CMD-SHELL)", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
    healthcheck:
      test: curl -f http://localhost
      interval: 15s
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		web := manifest.Services["web"]
		if web.Healthcheck == nil {
			t.Fatal("expected healthcheck to be set")
		}
		if len(web.Healthcheck.Test) != 1 || web.Healthcheck.Test[0] != "curl -f http://localhost" {
			t.Errorf("expected test ['curl -f http://localhost'], got %v", web.Healthcheck.Test)
		}
		if web.Healthcheck.Interval != "15s" {
			t.Errorf("expected interval '15s', got %q", web.Healthcheck.Interval)
		}
	})

	t.Run("disable healthcheck", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
    healthcheck:
      disable: true
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		web := manifest.Services["web"]
		if web.Healthcheck == nil {
			t.Fatal("expected healthcheck to be set")
		}
		if !web.Healthcheck.Disable {
			t.Error("expected disable to be true")
		}
	})

	t.Run("no healthcheck is nil", func(t *testing.T) {
		input := `
name: my-app
services:
  web:
    image: nginx
`
		var manifest BanyanManifest
		if err := yaml.Unmarshal([]byte(input), &manifest); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		web := manifest.Services["web"]
		if web.Healthcheck != nil {
			t.Error("expected nil healthcheck")
		}
	})
}
