package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
)

// EngineClient is an HTTP client for the Engine API.
type EngineClient struct {
	BaseURL  string
	Password string
	Client   *http.Client
}

// NewEngineClient creates a new engine API client.
func NewEngineClient(baseURL, password string) *EngineClient {
	return &EngineClient{
		BaseURL:  baseURL,
		Password: password,
		Client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Deploy creates a new deployment via the engine API.
func (c *EngineClient) Deploy(ctx context.Context, manifest types.BanyanManifest) (*types.DeployResponse, error) {
	req := types.DeployRequest{Manifest: manifest}
	var resp types.DeployResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/deploy", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Down stops services in a deployment via the engine API.
func (c *EngineClient) Down(ctx context.Context, name string, services []string) (*types.DownResponse, error) {
	req := types.DownRequest{Name: name, Services: services}
	var resp types.DownResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/down", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Status retrieves cluster status from the engine API.
func (c *EngineClient) Status(ctx context.Context) (*types.StatusResponse, error) {
	var resp types.StatusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/status", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Info retrieves engine info (registry URL, API port).
func (c *EngineClient) Info(ctx context.Context) (*types.InfoResponse, error) {
	var resp types.InfoResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/info", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StreamLogs streams logs for a container from the engine API.
func (c *EngineClient) StreamLogs(ctx context.Context, container string, follow bool, tail int) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/v1/logs/%s?follow=%t&tail=%d", c.BaseURL, container, follow, tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.Password != "" {
		req.SetBasicAuth("banyan", c.Password)
	}

	// Use a client without timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to engine: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("engine returned status %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// doJSON performs a JSON request to the engine API.
func (c *EngineClient) doJSON(ctx context.Context, method, path string, body interface{}, dest interface{}) error {
	url := c.BaseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Password != "" {
		req.SetBasicAuth("banyan", c.Password)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp types.ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("engine returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if dest != nil {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
