// Package dashboard implements a live terminal dashboard for Banyan cluster monitoring.
package dashboard

import (
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// DashboardData holds the view-layer representation of cluster state.
type DashboardData struct {
	FetchedAt   time.Time
	Agents      []AgentData
	Deployments []DeploymentData
	Events      []EventData
	Engine      EngineData
	Summary     ClusterSummary
}

// EngineData holds engine status and resource metrics.
type EngineData struct {
	StartedAt time.Time
	Status    string
	CPU       float64
	MemUsed   uint64
	MemTotal  uint64
	DiskUsed  uint64
	DiskTotal uint64
	CPUCores  uint32
}

// AgentData holds per-agent status and resource metrics.
type AgentData struct {
	LastSeen       time.Time
	CreatedAt      time.Time
	APIAddress     string
	Name           string
	VPCSubnet      string
	Status         string
	Tags           []string
	CPU            float64
	MemUsed        uint64
	MemTotal       uint64
	DiskUsed       uint64
	DiskTotal      uint64
	CPUCores       uint32
	ContainerCount int32
}

// DeploymentData holds deployment status and service details.
type DeploymentData struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	ID        string
	Name      string
	Status    string
	Error     string
	Tags      []string
	Services  int
	Healthy   int32
	Total     int32
}

// ClusterSummary holds aggregated cluster statistics.
type ClusterSummary struct {
	TotalAgents        int32
	ConnectedAgents    int32
	TotalDeployments   int32
	RunningDeployments int32
	TotalContainers    int32
	HealthyContainers  int32
	CompletedTasks     int32
	FailedTasks        int32
}

// EventData holds a single cluster event for display.
type EventData struct {
	Timestamp time.Time
	Type      string
	Message   string
	Severity  string
}

// ConvertFromProto converts a gRPC response to view-layer types.
func ConvertFromProto(resp *banyanpb.GetDashboardDataResponse) DashboardData {
	data := DashboardData{FetchedAt: time.Now()}

	// Engine
	if resp.Engine != nil {
		data.Engine = EngineData{
			Status:    resp.Engine.Status,
			StartedAt: time.Unix(resp.Engine.StartedAtUnix, 0),
		}
		if m := resp.Engine.SystemMetrics; m != nil {
			data.Engine.CPU = m.CpuUsageRatio
			data.Engine.MemUsed = m.MemoryUsedBytes
			data.Engine.MemTotal = m.MemoryTotalBytes
			data.Engine.DiskUsed = m.DiskUsedBytes
			data.Engine.DiskTotal = m.DiskTotalBytes
			data.Engine.CPUCores = m.CpuCores
		}
	}

	// Agents
	for _, a := range resp.Agents {
		ad := AgentData{
			Name:           a.Name,
			Status:         a.Status,
			Tags:           a.Tags,
			LastSeen:       time.Unix(a.LastSeenUnix, 0),
			CreatedAt:      time.Unix(a.CreatedAtUnix, 0),
			VPCSubnet:      a.VpcSubnet,
			APIAddress:     a.ApiAddress,
			ContainerCount: a.ContainerCount,
		}
		if m := a.SystemMetrics; m != nil {
			ad.CPU = m.CpuUsageRatio
			ad.MemUsed = m.MemoryUsedBytes
			ad.MemTotal = m.MemoryTotalBytes
			ad.DiskUsed = m.DiskUsedBytes
			ad.DiskTotal = m.DiskTotalBytes
			ad.CPUCores = m.CpuCores
		}
		data.Agents = append(data.Agents, ad)
	}

	// Deployments
	for _, d := range resp.Deployments {
		data.Deployments = append(data.Deployments, DeploymentData{
			ID:        d.Id,
			Name:      d.Name,
			Status:    d.Status,
			Healthy:   d.Healthy,
			Total:     d.Total,
			Services:  len(d.Services),
			Tags:      d.Tags,
			CreatedAt: time.Unix(d.CreatedAtUnix, 0),
			UpdatedAt: time.Unix(d.UpdatedAtUnix, 0),
			Error:     d.Error,
		})
	}

	// Summary
	if s := resp.Summary; s != nil {
		data.Summary = ClusterSummary{
			TotalAgents:        s.TotalAgents,
			ConnectedAgents:    s.ConnectedAgents,
			TotalDeployments:   s.TotalDeployments,
			RunningDeployments: s.RunningDeployments,
			TotalContainers:    s.TotalContainers,
			HealthyContainers:  s.HealthyContainers,
			CompletedTasks:     s.TasksByStatus["completed"],
			FailedTasks:        s.TasksByStatus["failed"],
		}
	}

	// Events
	for _, e := range resp.RecentEvents {
		data.Events = append(data.Events, EventData{
			Timestamp: time.Unix(e.TimestampUnix, 0),
			Type:      e.Type,
			Message:   e.Message,
			Severity:  e.Severity,
		})
	}

	return data
}
