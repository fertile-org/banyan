package types

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvFile supports both string and list forms in YAML, matching Docker Compose syntax.
//
//	env_file: .env          # string form
//	env_file:               # list form
//	  - .env
//	  - .env.local
type EnvFile []string

// UnmarshalYAML supports both string and sequence forms for env_file.
func (e *EnvFile) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*e = EnvFile{value.Value}
		return nil
	}
	if value.Kind == yaml.SequenceNode {
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*e = EnvFile(list)
		return nil
	}
	return fmt.Errorf("env_file must be a string or list of strings")
}

// ParseEnvFile reads a .env file and returns KEY=VALUE pairs.
// Handles: blank lines, # comments, KEY=VALUE, KEY="VALUE", KEY='VALUE',
// export KEY=VALUE, leading/trailing whitespace.
func ParseEnvFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file %s: %w", path, err)
	}
	defer f.Close()

	var result []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip "export " prefix
		line = strings.TrimPrefix(line, "export ")

		// Must contain =
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		// Strip inline comments (only when not inside quotes)
		if val != "" && val[0] != '"' && val[0] != '\'' {
			if commentIdx := strings.Index(val, " #"); commentIdx >= 0 {
				val = strings.TrimSpace(val[:commentIdx])
			}
		}

		// Strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if key == "" {
			continue
		}

		result = append(result, key+"="+val)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read env file %s: %w", path, err)
	}

	return result, nil
}

// ResolveEnvFiles loads env_file entries for all services, merging them
// with inline environment (inline takes precedence).
// manifestDir is used to resolve relative paths.
func ResolveEnvFiles(manifestDir string, services map[string]ManifestService) error {
	for name, svc := range services { //nolint:gocritic // map iteration with mutation
		if len(svc.EnvFile) == 0 {
			continue
		}

		// Build merged env map: env_file first, then inline overrides
		merged := make(map[string]string)

		// Load env files in order (later files override earlier)
		for _, envPath := range svc.EnvFile {
			if !filepath.IsAbs(envPath) {
				envPath = filepath.Join(manifestDir, envPath)
			}
			pairs, err := ParseEnvFile(envPath)
			if err != nil {
				return fmt.Errorf("service %q: %w", name, err)
			}
			for _, pair := range pairs {
				k, v := splitEnvPair(pair)
				merged[k] = v
			}
		}

		// Inline environment overrides env_file
		for _, pair := range svc.Environment {
			k, v := splitEnvPair(pair)
			merged[k] = v
		}

		// Convert back to sorted slice
		env := make([]string, 0, len(merged))
		for k, v := range merged {
			env = append(env, k+"="+v)
		}
		sort.Strings(env)

		svc.Environment = env
		svc.EnvFile = nil // resolved
		services[name] = svc
	}

	return nil
}

// splitEnvPair splits "KEY=VALUE" into key and value.
func splitEnvPair(pair string) (string, string) {
	if idx := strings.Index(pair, "="); idx >= 0 {
		return pair[:idx], pair[idx+1:]
	}
	return pair, ""
}
