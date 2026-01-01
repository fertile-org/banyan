// Package adapters provides implementations of the outbound ports for the Compose Parser.
package adapters

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// YAMLParserAdapter implements YAMLParser using gopkg.in/yaml.v3.
type YAMLParserAdapter struct{}

// NewYAMLParserAdapter creates a new YAMLParserAdapter.
func NewYAMLParserAdapter() *YAMLParserAdapter {
	return &YAMLParserAdapter{}
}

// ParseYAML parses YAML content into a generic map.
func (a *YAMLParserAdapter) ParseYAML(content string) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("yaml parse error: %w", err)
	}
	return result, nil
}

// UnmarshalYAML unmarshals YAML into a specific type.
func (a *YAMLParserAdapter) UnmarshalYAML(content string, v interface{}) error {
	if err := yaml.Unmarshal([]byte(content), v); err != nil {
		return fmt.Errorf("yaml unmarshal error: %w", err)
	}
	return nil
}
