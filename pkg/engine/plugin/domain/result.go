// Package domain contains the core domain entities for the Plugin Manager.
package domain

import "time"

// PluginResult represents the outcome of plugin execution.
type PluginResult struct {
	PluginName string
	Hook       HookPoint
	Success    bool
	Message    string
	Error      error
	Duration   time.Duration
	Output     map[string]any // Plugin-specific output
	Mutations  []Mutation     // Requested spec changes
}

// Mutation represents a change to the service spec.
type Mutation struct {
	Path      string // JSON path (e.g., "spec.replicas")
	Operation string // "set", "add", "remove"
	Value     any
}

// MutationOperation constants.
const (
	MutationOpSet    = "set"
	MutationOpAdd    = "add"
	MutationOpRemove = "remove"
)

// IsValidOperation checks if the mutation operation is valid.
func (m *Mutation) IsValidOperation() bool {
	switch m.Operation {
	case MutationOpSet, MutationOpAdd, MutationOpRemove:
		return true
	default:
		return false
	}
}

// HookResults aggregates results from multiple plugins.
type HookResults struct {
	Hook      HookPoint
	Results   []PluginResult
	AllPassed bool
	Duration  time.Duration
}

// FailedResults returns only the failed plugin results.
func (hr *HookResults) FailedResults() []PluginResult {
	var failed []PluginResult
	for _, r := range hr.Results {
		if !r.Success {
			failed = append(failed, r)
		}
	}
	return failed
}

// SuccessfulResults returns only the successful plugin results.
func (hr *HookResults) SuccessfulResults() []PluginResult {
	var successful []PluginResult
	for _, r := range hr.Results {
		if r.Success {
			successful = append(successful, r)
		}
	}
	return successful
}

// AllMutations collects all mutations from successful plugin results.
func (hr *HookResults) AllMutations() []Mutation {
	var mutations []Mutation
	for _, r := range hr.Results {
		if r.Success && len(r.Mutations) > 0 {
			mutations = append(mutations, r.Mutations...)
		}
	}
	return mutations
}

// ErrorMessages returns all error messages from failed results.
func (hr *HookResults) ErrorMessages() []string {
	var messages []string
	for _, r := range hr.Results {
		if !r.Success && r.Error != nil {
			messages = append(messages, r.Error.Error())
		}
	}
	return messages
}
