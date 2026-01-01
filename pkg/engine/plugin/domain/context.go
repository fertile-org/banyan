// Package domain contains the core domain entities for the Plugin Manager.
package domain

// ExecutionContext provides context to plugins during execution.
type ExecutionContext struct {
	Hook         HookPoint
	DeploymentID string
	ServiceName  string
	Namespace    string
	ServiceSpec  *ServiceSpec // Full service specification
	PreviousSpec *ServiceSpec // For updates, the previous spec
	Error        error        // For on-failure hook
	Metadata     map[string]string
}

// ServiceSpec represents a simplified service specification for plugins.
type ServiceSpec struct {
	Name       string
	Image      string
	Replicas   int
	Containers []ContainerSpec
	Labels     map[string]string
	Resources  ResourceRequirements
}

// ContainerSpec represents a container specification.
type ContainerSpec struct {
	Name      string
	Image     string
	Command   []string
	Args      []string
	Env       map[string]string
	Resources ResourceRequirements
}

// ResourceRequirements defines resource limits and requests.
type ResourceRequirements struct {
	CPULimit      float64 // CPU cores
	CPURequest    float64
	MemoryLimitMB int64 // Memory in MB
	MemoryRequest int64
}

// Clone creates a deep copy of the execution context.
func (ec *ExecutionContext) Clone() *ExecutionContext {
	clone := &ExecutionContext{
		Hook:         ec.Hook,
		DeploymentID: ec.DeploymentID,
		ServiceName:  ec.ServiceName,
		Namespace:    ec.Namespace,
		Error:        ec.Error,
	}

	if ec.ServiceSpec != nil {
		clone.ServiceSpec = ec.ServiceSpec.Clone()
	}

	if ec.PreviousSpec != nil {
		clone.PreviousSpec = ec.PreviousSpec.Clone()
	}

	if ec.Metadata != nil {
		clone.Metadata = make(map[string]string, len(ec.Metadata))
		for k, v := range ec.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// Clone creates a deep copy of the service spec.
func (ss *ServiceSpec) Clone() *ServiceSpec {
	clone := &ServiceSpec{
		Name:      ss.Name,
		Image:     ss.Image,
		Replicas:  ss.Replicas,
		Resources: ss.Resources,
	}

	if ss.Containers != nil {
		clone.Containers = make([]ContainerSpec, len(ss.Containers))
		for i := range ss.Containers {
			clone.Containers[i] = ss.Containers[i].Clone()
		}
	}

	if ss.Labels != nil {
		clone.Labels = make(map[string]string, len(ss.Labels))
		for k, v := range ss.Labels {
			clone.Labels[k] = v
		}
	}

	return clone
}

// Clone creates a deep copy of the container spec.
func (cs *ContainerSpec) Clone() ContainerSpec {
	clone := ContainerSpec{
		Name:      cs.Name,
		Image:     cs.Image,
		Resources: cs.Resources,
	}

	if cs.Command != nil {
		clone.Command = make([]string, len(cs.Command))
		copy(clone.Command, cs.Command)
	}

	if cs.Args != nil {
		clone.Args = make([]string, len(cs.Args))
		copy(clone.Args, cs.Args)
	}

	if cs.Env != nil {
		clone.Env = make(map[string]string, len(cs.Env))
		for k, v := range cs.Env {
			clone.Env[k] = v
		}
	}

	return clone
}
