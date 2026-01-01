// Package adapters provides implementations of the outbound ports for the Compose Parser.
package adapters

import (
	"os"
	"regexp"
	"strings"
)

// EnvInterpolatorAdapter implements EnvironmentInterpolator.
type EnvInterpolatorAdapter struct {
	envPattern *regexp.Regexp
}

// NewEnvInterpolatorAdapter creates a new EnvInterpolatorAdapter.
func NewEnvInterpolatorAdapter() *EnvInterpolatorAdapter {
	// Match ${VAR}, ${VAR:-default}, ${VAR-default}, $VAR
	pattern := regexp.MustCompile(`\$\{([^}:]+)(?::?-([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
	return &EnvInterpolatorAdapter{
		envPattern: pattern,
	}
}

// Interpolate replaces environment variables in content.
func (a *EnvInterpolatorAdapter) Interpolate(content string, env map[string]string) (string, error) {
	result := a.envPattern.ReplaceAllStringFunc(content, func(match string) string {
		// Handle ${VAR} or ${VAR:-default} format
		if strings.HasPrefix(match, "${") {
			inner := match[2 : len(match)-1]

			// Check for default value
			var varName, defaultVal string
			var hasDefault bool

			if idx := strings.Index(inner, ":-"); idx != -1 {
				varName = inner[:idx]
				defaultVal = inner[idx+2:]
				hasDefault = true
			} else if idx := strings.Index(inner, "-"); idx != -1 {
				varName = inner[:idx]
				defaultVal = inner[idx+1:]
				hasDefault = true
			} else {
				varName = inner
			}

			// Look up value
			if val, exists := env[varName]; exists {
				return val
			}
			if val, exists := os.LookupEnv(varName); exists {
				return val
			}
			if hasDefault {
				return defaultVal
			}
			return match // Keep original if not found
		}

		// Handle $VAR format
		varName := match[1:]
		if val, exists := env[varName]; exists {
			return val
		}
		if val, exists := os.LookupEnv(varName); exists {
			return val
		}
		return match
	})

	return result, nil
}
