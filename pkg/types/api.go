package types

// DeployRequest is the request body for POST /api/v1/deploy.
type DeployRequest struct {
	Manifest BanyanManifest `json:"manifest"`
}

// DeployResponse is the response body for POST /api/v1/deploy.
type DeployResponse struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
}

// DownRequest is the request body for POST /api/v1/down.
type DownRequest struct {
	Name     string   `json:"name"`
	Services []string `json:"services,omitempty"`
}

// DownResponse is the response body for POST /api/v1/down.
type DownResponse struct {
	TaskCount int `json:"task_count"`
}

// StatusResponse is the response body for GET /api/v1/status.
type StatusResponse struct {
	Agents      []NodeRecord       `json:"agents"`
	Deployments []DeploymentStatus `json:"deployments"`
}

// DeploymentStatus represents a deployment with its tasks and health info.
type DeploymentStatus struct {
	DeploymentRecord
	Tasks   []TaskRecord `json:"tasks,omitempty"`
	Healthy int          `json:"healthy"`
	Total   int          `json:"total"`
}

// InfoResponse is the response body for GET /api/v1/info.
type InfoResponse struct {
	RegistryURL string `json:"registry_url"`
	APIPort     string `json:"api_port"`
}

// HealthResponse is the response body for GET /api/v1/health.
type HealthResponse struct {
	Status string `json:"status"`
}

// ErrorResponse is returned for API errors.
type ErrorResponse struct {
	Error string `json:"error"`
}
