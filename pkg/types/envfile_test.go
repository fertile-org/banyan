package types

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnvFileUnmarshalYAML(t *testing.T) {
	t.Run("string form", func(t *testing.T) {
		input := `env_file: .env`
		var svc struct {
			EnvFile EnvFile `yaml:"env_file"`
		}
		if err := yaml.Unmarshal([]byte(input), &svc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if len(svc.EnvFile) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(svc.EnvFile))
		}
		if svc.EnvFile[0] != ".env" {
			t.Errorf("expected '.env', got %q", svc.EnvFile[0])
		}
	})

	t.Run("list form", func(t *testing.T) {
		input := "env_file:\n  - .env\n  - .env.local\n"
		var svc struct {
			EnvFile EnvFile `yaml:"env_file"`
		}
		if err := yaml.Unmarshal([]byte(input), &svc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if len(svc.EnvFile) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(svc.EnvFile))
		}
		if svc.EnvFile[0] != ".env" {
			t.Errorf("expected '.env', got %q", svc.EnvFile[0])
		}
		if svc.EnvFile[1] != ".env.local" {
			t.Errorf("expected '.env.local', got %q", svc.EnvFile[1])
		}
	})

	t.Run("invalid node type errors", func(t *testing.T) {
		input := `env_file:
  key: value`
		var svc struct {
			EnvFile EnvFile `yaml:"env_file"`
		}
		err := yaml.Unmarshal([]byte(input), &svc)
		if err == nil {
			t.Fatal("expected error for mapping node")
		}
	})

	t.Run("empty omitted", func(t *testing.T) {
		input := `image: nginx`
		var svc struct {
			EnvFile EnvFile `yaml:"env_file"`
		}
		if err := yaml.Unmarshal([]byte(input), &svc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if len(svc.EnvFile) != 0 {
			t.Errorf("expected empty, got %v", svc.EnvFile)
		}
	})
}

func TestParseEnvFile(t *testing.T) {
	writeEnv := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		return path
	}

	t.Run("basic KEY=VALUE", func(t *testing.T) {
		path := writeEnv(t, "FOO=bar\nBAZ=qux\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 2 {
			t.Fatalf("expected 2 pairs, got %d", len(pairs))
		}
		if pairs[0] != "FOO=bar" {
			t.Errorf("expected 'FOO=bar', got %q", pairs[0])
		}
		if pairs[1] != "BAZ=qux" {
			t.Errorf("expected 'BAZ=qux', got %q", pairs[1])
		}
	})

	t.Run("comments and blank lines", func(t *testing.T) {
		path := writeEnv(t, "# comment\n\nFOO=bar\n  # indented comment\n\nBAZ=qux\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 2 {
			t.Fatalf("expected 2 pairs, got %d: %v", len(pairs), pairs)
		}
	})

	t.Run("double quoted values", func(t *testing.T) {
		path := writeEnv(t, `DB_URL="postgres://host:5432/db"`)
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != "DB_URL=postgres://host:5432/db" {
			t.Errorf("expected unquoted value, got %q", pairs[0])
		}
	})

	t.Run("single quoted values", func(t *testing.T) {
		path := writeEnv(t, "SECRET='my secret value'")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != "SECRET=my secret value" {
			t.Errorf("expected unquoted value, got %q", pairs[0])
		}
	})

	t.Run("export prefix", func(t *testing.T) {
		path := writeEnv(t, "export FOO=bar\nexport BAZ=qux\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 2 {
			t.Fatalf("expected 2 pairs, got %d", len(pairs))
		}
		if pairs[0] != "FOO=bar" {
			t.Errorf("expected 'FOO=bar', got %q", pairs[0])
		}
	})

	t.Run("trailing whitespace", func(t *testing.T) {
		path := writeEnv(t, "  FOO = bar  \n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != "FOO=bar" {
			t.Errorf("expected 'FOO=bar', got %q", pairs[0])
		}
	})

	t.Run("inline comments", func(t *testing.T) {
		path := writeEnv(t, "FOO=bar # this is a comment\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != "FOO=bar" {
			t.Errorf("expected 'FOO=bar', got %q", pairs[0])
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := ParseEnvFile("/nonexistent/.env")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("empty value", func(t *testing.T) {
		path := writeEnv(t, "FOO=\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != "FOO=" {
			t.Errorf("expected 'FOO=', got %q", pairs[0])
		}
	})

	t.Run("value with equals sign", func(t *testing.T) {
		path := writeEnv(t, "CONNECTION=postgres://user:pass@host/db?sslmode=require\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d", len(pairs))
		}
		if pairs[0] != "CONNECTION=postgres://user:pass@host/db?sslmode=require" {
			t.Errorf("unexpected pair: %q", pairs[0])
		}
	})

	t.Run("line without equals skipped", func(t *testing.T) {
		path := writeEnv(t, "NOEQUALS\nFOO=bar\n")
		pairs, err := ParseEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pairs) != 1 {
			t.Fatalf("expected 1 pair, got %d: %v", len(pairs), pairs)
		}
		if pairs[0] != "FOO=bar" {
			t.Errorf("expected 'FOO=bar', got %q", pairs[0])
		}
	})
}

func TestResolveEnvFiles(t *testing.T) {
	writeEnv := func(t *testing.T, dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	t.Run("env_file only", func(t *testing.T) {
		dir := t.TempDir()
		writeEnv(t, dir, ".env", "FOO=bar\nDB=postgres\n")

		services := map[string]ManifestService{
			"web": {
				Image:   "nginx",
				EnvFile: EnvFile{".env"},
			},
		}

		if err := ResolveEnvFiles(dir, services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		svc := services["web"]
		if len(svc.Environment) != 2 {
			t.Fatalf("expected 2 env vars, got %d: %v", len(svc.Environment), svc.Environment)
		}
		// Sorted: DB=postgres, FOO=bar
		if svc.Environment[0] != "DB=postgres" {
			t.Errorf("expected 'DB=postgres', got %q", svc.Environment[0])
		}
		if svc.Environment[1] != "FOO=bar" {
			t.Errorf("expected 'FOO=bar', got %q", svc.Environment[1])
		}
		if len(svc.EnvFile) != 0 {
			t.Errorf("expected EnvFile cleared, got %v", svc.EnvFile)
		}
	})

	t.Run("inline only unchanged", func(t *testing.T) {
		services := map[string]ManifestService{
			"web": {
				Image:       "nginx",
				Environment: []string{"FOO=bar"},
			},
		}

		if err := ResolveEnvFiles("/tmp", services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		svc := services["web"]
		if len(svc.Environment) != 1 || svc.Environment[0] != "FOO=bar" {
			t.Errorf("expected unchanged env, got %v", svc.Environment)
		}
	})

	t.Run("inline overrides env_file", func(t *testing.T) {
		dir := t.TempDir()
		writeEnv(t, dir, ".env", "FOO=from-file\nBAR=from-file\n")

		services := map[string]ManifestService{
			"web": {
				Image:       "nginx",
				EnvFile:     EnvFile{".env"},
				Environment: []string{"FOO=from-inline"},
			},
		}

		if err := ResolveEnvFiles(dir, services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		svc := services["web"]
		if len(svc.Environment) != 2 {
			t.Fatalf("expected 2 env vars, got %d: %v", len(svc.Environment), svc.Environment)
		}
		// Sorted: BAR=from-file, FOO=from-inline
		if svc.Environment[0] != "BAR=from-file" {
			t.Errorf("expected 'BAR=from-file', got %q", svc.Environment[0])
		}
		if svc.Environment[1] != "FOO=from-inline" {
			t.Errorf("expected 'FOO=from-inline', got %q", svc.Environment[1])
		}
	})

	t.Run("multiple env files later overrides earlier", func(t *testing.T) {
		dir := t.TempDir()
		writeEnv(t, dir, ".env", "FOO=first\nBAR=first\n")
		writeEnv(t, dir, ".env.local", "FOO=second\n")

		services := map[string]ManifestService{
			"web": {
				Image:   "nginx",
				EnvFile: EnvFile{".env", ".env.local"},
			},
		}

		if err := ResolveEnvFiles(dir, services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		svc := services["web"]
		if len(svc.Environment) != 2 {
			t.Fatalf("expected 2 env vars, got %d: %v", len(svc.Environment), svc.Environment)
		}
		// Sorted: BAR=first, FOO=second
		if svc.Environment[0] != "BAR=first" {
			t.Errorf("expected 'BAR=first', got %q", svc.Environment[0])
		}
		if svc.Environment[1] != "FOO=second" {
			t.Errorf("expected 'FOO=second', got %q", svc.Environment[1])
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		dir := t.TempDir()
		envPath := filepath.Join(dir, "custom.env")
		if err := os.WriteFile(envPath, []byte("KEY=val\n"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}

		services := map[string]ManifestService{
			"web": {
				Image:   "nginx",
				EnvFile: EnvFile{envPath},
			},
		}

		if err := ResolveEnvFiles("/some/other/dir", services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		svc := services["web"]
		if len(svc.Environment) != 1 || svc.Environment[0] != "KEY=val" {
			t.Errorf("expected ['KEY=val'], got %v", svc.Environment)
		}
	})

	t.Run("missing env file returns error", func(t *testing.T) {
		services := map[string]ManifestService{
			"web": {
				Image:   "nginx",
				EnvFile: EnvFile{".env.nonexistent"},
			},
		}

		err := ResolveEnvFiles("/tmp", services)
		if err == nil {
			t.Fatal("expected error for missing env file")
		}
	})

	t.Run("service without env_file unchanged", func(t *testing.T) {
		services := map[string]ManifestService{
			"web": {
				Image:       "nginx",
				Environment: []string{"FOO=bar"},
			},
			"api": {
				Image: "node",
			},
		}

		if err := ResolveEnvFiles("/tmp", services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if services["web"].Environment[0] != "FOO=bar" {
			t.Error("web env should be unchanged")
		}
		if len(services["api"].Environment) != 0 {
			t.Error("api env should be empty")
		}
	})
}

func TestSplitEnvPair(t *testing.T) {
	t.Run("normal pair", func(t *testing.T) {
		k, v := splitEnvPair("FOO=bar")
		if k != "FOO" || v != "bar" {
			t.Errorf("expected FOO/bar, got %q/%q", k, v)
		}
	})

	t.Run("value with equals", func(t *testing.T) {
		k, v := splitEnvPair("URL=postgres://host?ssl=true")
		if k != "URL" || v != "postgres://host?ssl=true" {
			t.Errorf("expected URL/postgres://host?ssl=true, got %q/%q", k, v)
		}
	})

	t.Run("no equals", func(t *testing.T) {
		k, v := splitEnvPair("NOVALUE")
		if k != "NOVALUE" || v != "" {
			t.Errorf("expected NOVALUE/'', got %q/%q", k, v)
		}
	})

	t.Run("empty value", func(t *testing.T) {
		k, v := splitEnvPair("EMPTY=")
		if k != "EMPTY" || v != "" {
			t.Errorf("expected EMPTY/'', got %q/%q", k, v)
		}
	})
}
