// Package dashboard implements a live terminal dashboard for Banyan cluster monitoring.
package dashboard

import (
	"sort"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
)

// DashboardData holds the view-layer representation of cluster state.
type DashboardData struct {
	FetchedAt   time.Time
	Agents      []AgentData
	Deployments []DeploymentData
	Containers  []ContainerData
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
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ID             string
	Name           string
	Status         string
	Error          string
	Tags           []string
	ServiceDetails []ServiceData
	Containers     []ContainerData
	Services       int
	Healthy        int32
	Total          int32
}

// ServiceData holds per-service details within a deployment.
type ServiceData struct {
	Name      string
	Image     string
	Ports     []string
	DependsOn []string
	Replicas  int32
}

// ContainerData holds per-container info derived from tasks.
type ContainerData struct {
	CreatedAt       time.Time
	UpdatedAt       time.Time
	TaskID          string
	Name            string
	ServiceName     string
	AgentName       string
	DeploymentName  string
	DeploymentID    string
	Status          string
	ContainerStatus string
	HealthStatus    string
	Image           string
	Ports           []string
	CPUPercent      float64
	MemUsed         uint64
	MemLimit        uint64
	ReplicaIndex    int32
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
		dd := DeploymentData{
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
		}

		// Service details (sorted by name for stable display)
		var serviceNames []string
		for name := range d.Services {
			serviceNames = append(serviceNames, name)
		}
		sort.Strings(serviceNames)
		for _, name := range serviceNames {
			svc := d.Services[name]
			dd.ServiceDetails = append(dd.ServiceDetails, ServiceData{
				Name:      name,
				Image:     svc.Image,
				Replicas:  svc.Replicas,
				Ports:     svc.Ports,
				DependsOn: protoDependsOnToNames(svc.DependsOn),
			})
		}

		// Container data from create_and_start tasks
		for _, t := range d.Tasks {
			if t.Type != "create_and_start" {
				continue
			}
			c := ContainerData{
				TaskID:          t.Id,
				Name:            t.ContainerName,
				ServiceName:     t.ServiceName,
				AgentName:       t.AgentId,
				DeploymentName:  d.Name,
				DeploymentID:    d.Id,
				Status:          t.Status,
				ContainerStatus: t.ContainerStatus,
				HealthStatus:    t.HealthStatus,
				Image:           t.Image,
				Ports:           t.Ports,
				CPUPercent:      t.CpuPercent,
				MemUsed:         t.MemoryUsedBytes,
				MemLimit:        t.MemoryLimitBytes,
				ReplicaIndex:    t.ReplicaIndex,
				CreatedAt:       time.Unix(t.CreatedAtUnix, 0),
				UpdatedAt:       time.Unix(t.UpdatedAtUnix, 0),
			}
			dd.Containers = append(dd.Containers, c)
			data.Containers = append(data.Containers, c)
		}

		data.Deployments = append(data.Deployments, dd)
	}

	// Sort global containers: deployment → service → replica
	sort.Slice(data.Containers, func(i, j int) bool {
		ci, cj := &data.Containers[i], &data.Containers[j]
		if ci.DeploymentName != cj.DeploymentName {
			return ci.DeploymentName < cj.DeploymentName
		}
		if ci.ServiceName != cj.ServiceName {
			return ci.ServiceName < cj.ServiceName
		}
		return ci.ReplicaIndex < cj.ReplicaIndex
	})

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

// protoDependsOnToNames extracts sorted dependency names from the proto map.
func protoDependsOnToNames(deps map[string]*banyanpb.DependsOnCondition) []string {
	if len(deps) == 0 {
		return nil
	}
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
