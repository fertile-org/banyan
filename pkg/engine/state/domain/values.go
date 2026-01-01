package domain

import "time"

// PortMapping defines port exposure.
type PortMapping struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

// ReconcileAction represents a corrective action.
type ReconcileAction struct {
	Type        ActionType
	ServiceName string
	AgentID     string
	Details     map[string]interface{}
}

// ActionType categorizes the type of reconcile action.
type ActionType string

const (
	ActionDeploy  ActionType = "deploy"
	ActionStop    ActionType = "stop"
	ActionRestart ActionType = "restart"
	ActionScale   ActionType = "scale"
	ActionMigrate ActionType = "migrate"
)

// DriftReport summarizes drift across all deployments.
type DriftReport struct {
	TotalDeployments     int
	DeploymentsWithDrift int
	CriticalDrifts       int
	HighDrifts           int
	MediumDrifts         int
	LowDrifts            int
	GeneratedAt          time.Time
}

// ReconcileStatus tracks reconciliation status for a deployment.
type ReconcileStatus struct {
	DeploymentID    string
	IsReconciling   bool
	LastReconcileAt time.Time
	LastError       string
}
