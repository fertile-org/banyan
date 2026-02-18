package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

// startEngineAPI starts the Engine HTTP API server in a goroutine.
func startEngineAPI(ctx context.Context, store *storage.EtcdStore, port, password, registryURL string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/deploy", handleDeploy(store))
	mux.HandleFunc("/api/v1/down", handleDown(store))
	mux.HandleFunc("/api/v1/status", handleStatus(store))
	mux.HandleFunc("/api/v1/logs/", handleLogs(store, password))
	mux.HandleFunc("/api/v1/info", handleInfo(registryURL, port))
	mux.HandleFunc("/api/v1/health", handleHealth())

	var handler http.Handler = mux
	if password != "" {
		handler = types.BasicAuthMiddleware(mux, password)
	}

	server := &http.Server{
		Addr:    net.JoinHostPort("", port),
		Handler: handler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Engine API server error: %v\n", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()
}

func handleDeploy(store *storage.EtcdStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, types.ErrorResponse{Error: "method not allowed"})
			return
		}

		var req types.DeployRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: "invalid request body: " + err.Error()})
			return
		}

		manifest := req.Manifest
		if manifest.Name == "" {
			writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: "manifest must have a name"})
			return
		}
		if len(manifest.Services) == 0 {
			writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: "manifest must define at least one service"})
			return
		}

		for name, svc := range manifest.Services {
			if svc.Image == "" && svc.Build == nil {
				writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: fmt.Sprintf("service %q must have either 'image' or 'build'", name)})
				return
			}
		}

		ctx := r.Context()
		deploymentID := fmt.Sprintf("%s-%d", manifest.Name, time.Now().Unix())
		services := types.BuildServiceRecords(manifest.Services)

		record := &types.DeploymentRecord{
			ID:        deploymentID,
			Name:      manifest.Name,
			Status:    types.StatusPending,
			Services:  services,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := store.Save(ctx, types.KeyDeployments+deploymentID, record); err != nil {
			writeJSON(w, http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create deployment: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, types.DeployResponse{
			DeploymentID: deploymentID,
			Status:       types.StatusPending,
		})
	}
}

func handleDown(store *storage.EtcdStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, types.ErrorResponse{Error: "method not allowed"})
			return
		}

		var req types.DownRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: "invalid request body: " + err.Error()})
			return
		}

		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: "name is required"})
			return
		}

		ctx := r.Context()

		deployment, deploymentKey, err := findDeploymentByName(ctx, store, req.Name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, types.ErrorResponse{Error: err.Error()})
			return
		}

		// Validate requested services exist
		if len(req.Services) > 0 {
			for _, name := range req.Services {
				if _, ok := deployment.Services[name]; !ok {
					writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: fmt.Sprintf("service %q not found in deployment %q", name, req.Name)})
					return
				}
			}
		}

		// Collect tasks
		allTasks := types.CollectDeploymentTasks(ctx, store, deployment.ID)

		var targetTasks []types.TaskRecord
		for _, task := range allTasks {
			if task.Type == types.TaskTypeCreateAndStart && task.Status == types.StatusCompleted {
				targetTasks = append(targetTasks, task)
			}
		}

		// Filter by service names if specified
		if len(req.Services) > 0 {
			serviceSet := make(map[string]bool, len(req.Services))
			for _, name := range req.Services {
				serviceSet[name] = true
			}
			var filtered []types.TaskRecord
			for _, task := range targetTasks {
				if serviceSet[task.ServiceName] {
					filtered = append(filtered, task)
				}
			}
			targetTasks = filtered
		}

		if len(targetTasks) == 0 {
			writeJSON(w, http.StatusOK, types.DownResponse{TaskCount: 0})
			return
		}

		// Create stop_and_remove tasks
		now := time.Now()
		for _, task := range targetTasks {
			stopTask := &types.TaskRecord{
				ID:            task.ID + "-stop",
				DeploymentID:  task.DeploymentID,
				ServiceName:   task.ServiceName,
				AgentID:       task.AgentID,
				Type:          types.TaskTypeStopAndRemove,
				Status:        types.StatusPending,
				ContainerName: task.ContainerName,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			taskKey := types.KeyTasks + task.AgentID + "/" + stopTask.ID
			if err := store.Save(ctx, taskKey, stopTask); err != nil {
				writeJSON(w, http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create stop task: " + err.Error()})
				return
			}
		}

		// Update deployment status if stopping all services
		if len(req.Services) == 0 {
			deployment.Status = types.StatusStopping
			deployment.UpdatedAt = time.Now()
			store.Save(ctx, deploymentKey, deployment)
		}

		writeJSON(w, http.StatusOK, types.DownResponse{TaskCount: len(targetTasks)})
	}
}

func handleStatus(store *storage.EtcdStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, types.ErrorResponse{Error: "method not allowed"})
			return
		}

		ctx := r.Context()

		// Collect agents
		agents, _ := listAvailableAgents(ctx, store)

		// Also include non-ready agents for the status response
		allAgentKeys, _ := store.List(ctx, types.KeyNodes)
		var allAgents []types.NodeRecord
		for _, key := range allAgentKeys {
			var node types.NodeRecord
			if err := store.Get(ctx, key, &node); err != nil {
				continue
			}
			allAgents = append(allAgents, node)
		}
		_ = agents // we already have allAgents

		// Collect deployments
		deployKeys, _ := store.List(ctx, types.KeyDeployments)
		var deployments []types.DeploymentStatus
		for _, key := range deployKeys {
			var record types.DeploymentRecord
			if err := store.Get(ctx, key, &record); err != nil {
				continue
			}

			allTasks := types.CollectDeploymentTasks(ctx, store, record.ID)
			var createTasks []types.TaskRecord
			for _, t := range allTasks {
				if t.Type == types.TaskTypeCreateAndStart {
					createTasks = append(createTasks, t)
				}
			}

			healthy := 0
			for _, t := range createTasks {
				if t.ContainerStatus == types.StatusRunning {
					healthy++
				}
			}

			deployments = append(deployments, types.DeploymentStatus{
				DeploymentRecord: record,
				Tasks:            allTasks,
				Healthy:          healthy,
				Total:            len(createTasks),
			})
		}

		writeJSON(w, http.StatusOK, types.StatusResponse{
			Agents:      allAgents,
			Deployments: deployments,
		})
	}
}

func handleLogs(store *storage.EtcdStore, password string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, types.ErrorResponse{Error: "method not allowed"})
			return
		}

		// Extract container name from path: /api/v1/logs/<container>
		containerName := strings.TrimPrefix(r.URL.Path, "/api/v1/logs/")
		containerName = strings.TrimSuffix(containerName, "/")
		if containerName == "" {
			writeJSON(w, http.StatusBadRequest, types.ErrorResponse{Error: "container name required"})
			return
		}

		ctx := r.Context()

		// Find which agent has this container
		task, node, err := findContainerAgent(ctx, store, containerName)
		if err != nil {
			writeJSON(w, http.StatusNotFound, types.ErrorResponse{Error: err.Error()})
			return
		}

		if node.APIAddress == "" {
			writeJSON(w, http.StatusServiceUnavailable, types.ErrorResponse{
				Error: fmt.Sprintf("container %s is on node %s but agent API is not available", containerName, task.AgentID),
			})
			return
		}

		// Proxy the request to the agent
		query := r.URL.Query()
		agentURL := fmt.Sprintf("http://%s/v1/logs/%s?follow=%s&tail=%s",
			node.APIAddress, containerName,
			query.Get("follow"), query.Get("tail"))

		proxyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, agentURL, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create proxy request"})
			return
		}

		if password != "" {
			proxyReq.SetBasicAuth("banyan", password)
		}

		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, types.ErrorResponse{Error: "failed to connect to agent: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(resp.StatusCode)

		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return
				}
				if canFlush {
					flusher.Flush()
				}
			}
			if readErr != nil {
				return
			}
		}
	}
}

func handleInfo(registryURL, apiPort string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, types.ErrorResponse{Error: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, types.InfoResponse{
			RegistryURL: registryURL,
			APIPort:     apiPort,
		})
	}
}

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, types.HealthResponse{Status: "ok"})
	}
}

// findContainerAgent scans etcd to find which agent has the given container.
func findContainerAgent(ctx context.Context, store *storage.EtcdStore, containerName string) (*types.TaskRecord, *types.NodeRecord, error) {
	nodeKeys, err := store.List(ctx, types.KeyNodes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, nodeKey := range nodeKeys {
		var node types.NodeRecord
		if err := store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskKeys, err := store.List(ctx, types.KeyTasks+node.Name+"/")
		if err != nil {
			continue
		}

		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.ContainerName == containerName && task.Type == types.TaskTypeCreateAndStart {
				return &task, &node, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("container %q not found in cluster", containerName)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
