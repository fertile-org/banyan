package domain

import (
	"testing"
)

func TestContainer_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name      string
		fromState ContainerState
		toState   ContainerState
		want      bool
	}{
		// From Created
		{
			name:      "created to running",
			fromState: ContainerStateCreated,
			toState:   ContainerStateRunning,
			want:      true,
		},
		{
			name:      "created to dead",
			fromState: ContainerStateCreated,
			toState:   ContainerStateDead,
			want:      true,
		},
		{
			name:      "created to paused - invalid",
			fromState: ContainerStateCreated,
			toState:   ContainerStatePaused,
			want:      false,
		},

		// From Running
		{
			name:      "running to paused",
			fromState: ContainerStateRunning,
			toState:   ContainerStatePaused,
			want:      true,
		},
		{
			name:      "running to exited",
			fromState: ContainerStateRunning,
			toState:   ContainerStateExited,
			want:      true,
		},
		{
			name:      "running to restarting",
			fromState: ContainerStateRunning,
			toState:   ContainerStateRestarting,
			want:      true,
		},
		{
			name:      "running to dead",
			fromState: ContainerStateRunning,
			toState:   ContainerStateDead,
			want:      true,
		},
		{
			name:      "running to created - invalid",
			fromState: ContainerStateRunning,
			toState:   ContainerStateCreated,
			want:      false,
		},

		// From Paused
		{
			name:      "paused to running",
			fromState: ContainerStatePaused,
			toState:   ContainerStateRunning,
			want:      true,
		},
		{
			name:      "paused to dead",
			fromState: ContainerStatePaused,
			toState:   ContainerStateDead,
			want:      true,
		},
		{
			name:      "paused to exited - invalid",
			fromState: ContainerStatePaused,
			toState:   ContainerStateExited,
			want:      false,
		},

		// From Restarting
		{
			name:      "restarting to running",
			fromState: ContainerStateRestarting,
			toState:   ContainerStateRunning,
			want:      true,
		},
		{
			name:      "restarting to exited",
			fromState: ContainerStateRestarting,
			toState:   ContainerStateExited,
			want:      true,
		},
		{
			name:      "restarting to dead",
			fromState: ContainerStateRestarting,
			toState:   ContainerStateDead,
			want:      true,
		},

		// From Exited
		{
			name:      "exited to running",
			fromState: ContainerStateExited,
			toState:   ContainerStateRunning,
			want:      true,
		},
		{
			name:      "exited to dead",
			fromState: ContainerStateExited,
			toState:   ContainerStateDead,
			want:      true,
		},
		{
			name:      "exited to paused - invalid",
			fromState: ContainerStateExited,
			toState:   ContainerStatePaused,
			want:      false,
		},

		// From Dead (terminal)
		{
			name:      "dead to running - invalid",
			fromState: ContainerStateDead,
			toState:   ContainerStateRunning,
			want:      false,
		},
		{
			name:      "dead to created - invalid",
			fromState: ContainerStateDead,
			toState:   ContainerStateCreated,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{State: tt.fromState}
			if got := c.CanTransitionTo(tt.toState); got != tt.want {
				t.Errorf("Container.CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainer_TransitionTo(t *testing.T) {
	tests := []struct {
		name      string
		fromState ContainerState
		toState   ContainerState
		wantErr   bool
	}{
		{
			name:      "valid transition",
			fromState: ContainerStateCreated,
			toState:   ContainerStateRunning,
			wantErr:   false,
		},
		{
			name:      "invalid transition",
			fromState: ContainerStateCreated,
			toState:   ContainerStatePaused,
			wantErr:   true,
		},
		{
			name:      "terminal state blocks transition",
			fromState: ContainerStateDead,
			toState:   ContainerStateRunning,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{State: tt.fromState}
			err := c.TransitionTo(tt.toState)
			if (err != nil) != tt.wantErr {
				t.Errorf("Container.TransitionTo() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && c.State != tt.toState {
				t.Errorf("Container.TransitionTo() state = %v, want %v", c.State, tt.toState)
			}
		})
	}
}

func TestResourceLimits_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		limits ResourceLimits
		want   bool
	}{
		{
			name:   "empty limits",
			limits: ResourceLimits{},
			want:   true,
		},
		{
			name: "with memory limit",
			limits: ResourceLimits{
				MemoryLimit: 1024,
			},
			want: false,
		},
		{
			name: "with CPU shares",
			limits: ResourceLimits{
				CPUShares: 512,
			},
			want: false,
		},
		{
			name: "with PIDs limit",
			limits: ResourceLimits{
				PidsLimit: 100,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.limits.IsEmpty(); got != tt.want {
				t.Errorf("ResourceLimits.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ContainerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ContainerConfig{
				Image: ImageRef{
					Registry: "docker.io",
					Name:     "library/nginx",
					Tag:      "latest",
				},
			},
			wantErr: false,
		},
		{
			name: "missing image",
			config: ContainerConfig{
				Image: ImageRef{},
			},
			wantErr: true,
		},
		{
			name: "negative memory limit",
			config: ContainerConfig{
				Image: ImageRef{
					Name: "nginx",
				},
				Resources: ResourceLimits{
					MemoryLimit: -1,
				},
			},
			wantErr: true,
		},
		{
			name: "negative CPU quota",
			config: ContainerConfig{
				Image: ImageRef{
					Name: "nginx",
				},
				Resources: ResourceLimits{
					CPUQuota: -1,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid mount",
			config: ContainerConfig{
				Image: ImageRef{
					Name: "nginx",
				},
				Mounts: []MountPoint{
					{
						Source:      "/host",
						Destination: "", // invalid - empty
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ContainerConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
