// Package adapters provides implementations of the outbound ports for the Plugin Manager.
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	pluginerrors "github.com/fertile-org/banyan/pkg/engine/plugin/errors"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// WebhookRunner executes webhook plugins.
type WebhookRunner struct {
	client *http.Client
	logger *slog.Logger
}

// Ensure WebhookRunner implements PluginRunner.
var _ outbound.PluginRunner = (*WebhookRunner)(nil)

// NewWebhookRunner creates a new WebhookRunner.
func NewWebhookRunner(timeout time.Duration, logger *slog.Logger) *WebhookRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookRunner{
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
}

// CanRun checks if the runner can handle this plugin type.
func (r *WebhookRunner) CanRun(plugin *domain.Plugin) bool {
	return plugin.Type == domain.PluginTypeWebhook
}

// Run executes a webhook plugin.
func (r *WebhookRunner) Run(
	ctx context.Context,
	plugin *domain.Plugin,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - interface requirement
) (*domain.PluginResult, error) {
	// Build request payload
	payload := WebhookPayload{
		Hook:         string(execCtx.Hook),
		DeploymentID: execCtx.DeploymentID,
		ServiceName:  execCtx.ServiceName,
		Namespace:    execCtx.Namespace,
		ServiceSpec:  execCtx.ServiceSpec,
		Metadata:     execCtx.Metadata,
	}

	if execCtx.Error != nil {
		payload.Error = execCtx.Error.Error()
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	method := plugin.Spec.WebhookMethod
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		plugin.Spec.WebhookURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range plugin.Spec.WebhookHeaders {
		req.Header.Set(k, v)
	}

	r.logger.Debug("executing webhook",
		"plugin", plugin.Name,
		"url", plugin.Spec.WebhookURL,
	)

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Error("webhook request failed",
			"plugin", plugin.Name,
			"error", err,
		)
		return nil, fmt.Errorf("%w: %v", pluginerrors.ErrWebhookFailed, err)
	}
	defer resp.Body.Close()

	// Parse response
	var response WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result := &domain.PluginResult{
		PluginName: plugin.Name,
		Hook:       execCtx.Hook,
		Success:    resp.StatusCode >= 200 && resp.StatusCode < 300 && response.Success,
		Message:    response.Message,
		Output:     response.Output,
		Mutations:  convertMutations(response.Mutations),
	}

	if !result.Success {
		result.Error = fmt.Errorf("webhook returned failure: %s", response.Message)
	}

	return result, nil
}

// WebhookPayload is the request sent to webhook plugins.
type WebhookPayload struct {
	Hook         string              `json:"hook"`
	DeploymentID string              `json:"deployment_id"`
	ServiceName  string              `json:"service_name"`
	Namespace    string              `json:"namespace"`
	ServiceSpec  *domain.ServiceSpec `json:"service_spec,omitempty"`
	Error        string              `json:"error,omitempty"`
	Metadata     map[string]string   `json:"metadata,omitempty"`
}

// WebhookResponse is the response from webhook plugins.
type WebhookResponse struct {
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	Output    map[string]any    `json:"output,omitempty"`
	Mutations []MutationRequest `json:"mutations,omitempty"`
}

// MutationRequest is a mutation request from a webhook.
type MutationRequest struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Value     any    `json:"value"`
}

// convertMutations converts MutationRequests to domain Mutations.
func convertMutations(requests []MutationRequest) []domain.Mutation {
	if len(requests) == 0 {
		return nil
	}

	mutations := make([]domain.Mutation, len(requests))
	for i, req := range requests {
		mutations[i] = domain.Mutation{
			Path:      req.Path,
			Operation: req.Operation,
			Value:     req.Value,
		}
	}
	return mutations
}
