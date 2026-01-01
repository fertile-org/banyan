package domain

import (
	"testing"
)

func TestContainerID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      ContainerID
		wantErr bool
	}{
		{
			name:    "valid container ID",
			id:      ContainerID("abc123def456"),
			wantErr: false,
		},
		{
			name:    "valid long container ID",
			id:      ContainerID("abc123def456789012345678901234567890"),
			wantErr: false,
		},
		{
			name:    "invalid short container ID",
			id:      ContainerID("abc"),
			wantErr: true,
		},
		{
			name:    "empty container ID",
			id:      ContainerID(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ContainerID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContainerID_Short(t *testing.T) {
	tests := []struct {
		name string
		id   ContainerID
		want string
	}{
		{
			name: "long ID returns first 12",
			id:   ContainerID("abc123def456789012345678901234567890"),
			want: "abc123def456",
		},
		{
			name: "exact 12 char ID",
			id:   ContainerID("abc123def456"),
			want: "abc123def456",
		},
		{
			name: "short ID returns full string",
			id:   ContainerID("abc"),
			want: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Short(); got != tt.want {
				t.Errorf("ContainerID.Short() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImageID_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      ImageID
		wantErr bool
	}{
		{
			name:    "valid image ID",
			id:      ImageID("sha256:abc123"),
			wantErr: false,
		},
		{
			name:    "empty image ID",
			id:      ImageID(""),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ImageID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		want     ImageRef
		wantErr  bool
	}{
		{
			name: "simple image name",
			ref:  "nginx",
			want: ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			wantErr: false,
		},
		{
			name: "image with tag",
			ref:  "nginx:1.21",
			want: ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "1.21",
			},
			wantErr: false,
		},
		{
			name: "image with namespace",
			ref:  "myuser/myimage:v1",
			want: ImageRef{
				Registry: "docker.io",
				Name:     "myuser/myimage",
				Tag:      "v1",
			},
			wantErr: false,
		},
		{
			name: "image with custom registry",
			ref:  "gcr.io/myproject/myimage:latest",
			want: ImageRef{
				Registry: "gcr.io",
				Name:     "myproject/myimage",
				Tag:      "latest",
			},
			wantErr: false,
		},
		{
			name: "image with digest",
			ref:  "nginx@sha256:abc123",
			want: ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
				Digest:   "sha256:abc123",
			},
			wantErr: false,
		},
		{
			name:    "empty string",
			ref:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseImageRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseImageRef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Registry != tt.want.Registry {
					t.Errorf("ParseImageRef() Registry = %v, want %v", got.Registry, tt.want.Registry)
				}
				if got.Name != tt.want.Name {
					t.Errorf("ParseImageRef() Name = %v, want %v", got.Name, tt.want.Name)
				}
				if got.Tag != tt.want.Tag {
					t.Errorf("ParseImageRef() Tag = %v, want %v", got.Tag, tt.want.Tag)
				}
				if got.Digest != tt.want.Digest {
					t.Errorf("ParseImageRef() Digest = %v, want %v", got.Digest, tt.want.Digest)
				}
			}
		})
	}
}

func TestImageRef_String(t *testing.T) {
	tests := []struct {
		name string
		ref  ImageRef
		want string
	}{
		{
			name: "simple image",
			ref: ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			want: "library/nginx:latest",
		},
		{
			name: "custom registry",
			ref: ImageRef{
				Registry: "gcr.io",
				Name:     "myproject/myimage",
				Tag:      "v1",
			},
			want: "gcr.io/myproject/myimage:v1",
		},
		{
			name: "with digest",
			ref: ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
				Digest:   "sha256:abc123",
			},
			want: "library/nginx@sha256:abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.String(); got != tt.want {
				t.Errorf("ImageRef.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImageRef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ref     ImageRef
		wantErr bool
	}{
		{
			name: "valid image ref",
			ref: ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			wantErr: false,
		},
		{
			name: "empty name",
			ref: ImageRef{
				Registry: "docker.io",
				Name:     "",
				Tag:      "latest",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ImageRef.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContainerState_IsRunning(t *testing.T) {
	tests := []struct {
		name  string
		state ContainerState
		want  bool
	}{
		{
			name:  "running state",
			state: ContainerStateRunning,
			want:  true,
		},
		{
			name:  "created state",
			state: ContainerStateCreated,
			want:  false,
		},
		{
			name:  "exited state",
			state: ContainerStateExited,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsRunning(); got != tt.want {
				t.Errorf("ContainerState.IsRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerState_IsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state ContainerState
		want  bool
	}{
		{
			name:  "exited state is terminal",
			state: ContainerStateExited,
			want:  true,
		},
		{
			name:  "dead state is terminal",
			state: ContainerStateDead,
			want:  true,
		},
		{
			name:  "running state is not terminal",
			state: ContainerStateRunning,
			want:  false,
		},
		{
			name:  "created state is not terminal",
			state: ContainerStateCreated,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.want {
				t.Errorf("ContainerState.IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMountPoint_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mount   MountPoint
		wantErr bool
	}{
		{
			name: "valid mount",
			mount: MountPoint{
				Source:      "/host/path",
				Destination: "/container/path",
				Type:        MountTypeBind,
			},
			wantErr: false,
		},
		{
			name: "empty destination",
			mount: MountPoint{
				Source:      "/host/path",
				Destination: "",
				Type:        MountTypeBind,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mount.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("MountPoint.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPortBinding_String(t *testing.T) {
	tests := []struct {
		name string
		port PortBinding
		want string
	}{
		{
			name: "simple port binding",
			port: PortBinding{
				ContainerPort: 80,
				HostPort:      8080,
				Protocol:      ProtocolTCP,
			},
			want: "8080->80/tcp",
		},
		{
			name: "with host IP",
			port: PortBinding{
				HostIP:        "127.0.0.1",
				ContainerPort: 80,
				HostPort:      8080,
				Protocol:      ProtocolTCP,
			},
			want: "127.0.0.1:8080->80/tcp",
		},
		{
			name: "UDP protocol",
			port: PortBinding{
				ContainerPort: 53,
				HostPort:      5353,
				Protocol:      ProtocolUDP,
			},
			want: "5353->53/udp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.port.String(); got != tt.want {
				t.Errorf("PortBinding.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvVar_String(t *testing.T) {
	env := EnvVar{Name: "FOO", Value: "bar"}
	if got := env.String(); got != "FOO=bar" {
		t.Errorf("EnvVar.String() = %v, want FOO=bar", got)
	}
}
