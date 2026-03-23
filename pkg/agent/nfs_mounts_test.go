package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestIsNFSVolume(t *testing.T) {
	tests := []struct {
		name string
		vol  types.VolumeMount
		want bool
	}{
		{"nfs type", types.VolumeMount{Type: "nfs"}, true},
		{"bind type", types.VolumeMount{Type: "bind"}, false},
		{"volume type", types.VolumeMount{Type: "volume"}, false},
		{"empty type", types.VolumeMount{Type: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNFSVolume(tt.vol); got != tt.want {
				t.Errorf("IsNFSVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveNFSVolumes_NoNFS(t *testing.T) {
	vols := []types.VolumeMount{
		{Type: "volume", Source: "data", Target: "/data"},
		{Type: "bind", Source: "/host", Target: "/container"},
	}
	resolved, err := ResolveNFSVolumes(context.Background(), vols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(resolved))
	}
	// Should be unchanged
	if resolved[0].Type != "volume" || resolved[1].Type != "bind" {
		t.Errorf("non-NFS volumes should be unchanged: %+v", resolved)
	}
}

func TestResolveNFSVolumes_WithNFS(t *testing.T) {
	// Mock mount commands
	origMount := mountCommand
	origIsMounted := isMountedCommand
	origBase := nfsMountBase
	t.Cleanup(func() {
		mountCommand = origMount
		isMountedCommand = origIsMounted
		nfsMountBase = origBase
	})

	nfsMountBase = t.TempDir()
	mountCommand = func(ctx context.Context, args ...string) error {
		return nil // pretend mount succeeded
	}
	isMountedCommand = func(path string) bool {
		return false // not yet mounted
	}

	vols := []types.VolumeMount{
		{Type: "volume", Source: "data", Target: "/data"},
		{Type: "nfs", Source: "nfs.internal:/exports/uploads", Target: "/app/uploads"},
	}
	resolved, err := ResolveNFSVolumes(context.Background(), vols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First volume unchanged
	if resolved[0].Type != "volume" {
		t.Errorf("first volume should be unchanged, got type %q", resolved[0].Type)
	}
	// NFS converted to bind mount
	if resolved[1].Type != "bind" {
		t.Errorf("NFS volume should be converted to bind, got type %q", resolved[1].Type)
	}
	if resolved[1].Target != "/app/uploads" {
		t.Errorf("target should be /app/uploads, got %q", resolved[1].Target)
	}
	// Source should be a local mount path
	if resolved[1].Source == "nfs.internal:/exports/uploads" {
		t.Errorf("source should be a local path, not NFS address")
	}
}

func TestResolveNFSVolumes_AlreadyMounted(t *testing.T) {
	origMount := mountCommand
	origIsMounted := isMountedCommand
	origBase := nfsMountBase
	t.Cleanup(func() {
		mountCommand = origMount
		isMountedCommand = origIsMounted
		nfsMountBase = origBase
	})

	nfsMountBase = t.TempDir()
	mountCalled := false
	mountCommand = func(ctx context.Context, args ...string) error {
		mountCalled = true
		return nil
	}
	isMountedCommand = func(path string) bool {
		return true // already mounted
	}

	vols := []types.VolumeMount{
		{Type: "nfs", Source: "nfs.internal:/data", Target: "/data"},
	}
	_, err := ResolveNFSVolumes(context.Background(), vols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mountCalled {
		t.Error("mount should not be called when already mounted")
	}
}

func TestResolveNFSVolumes_EmptySource(t *testing.T) {
	origIsMounted := isMountedCommand
	t.Cleanup(func() { isMountedCommand = origIsMounted })
	isMountedCommand = func(path string) bool { return false }

	vols := []types.VolumeMount{
		{Type: "nfs", Source: "", Target: "/data"},
	}
	_, err := ResolveNFSVolumes(context.Background(), vols)
	if err == nil {
		t.Fatal("expected error for empty NFS source")
	}
}

func TestResolveTopLevelNFSVolumes(t *testing.T) {
	// This function is in nfs_mounts.go but tested here
	topLevel := map[string]types.VolumeConfig{
		"uploads": {
			Driver: "local",
			DriverOpts: map[string]string{
				"type":   "nfs",
				"o":      "addr=nfs.internal,vers=4,soft",
				"device": ":/exports/uploads",
			},
		},
		"local-data": {
			Driver: "local",
		},
	}

	mounts := []types.VolumeMount{
		{Type: "volume", Source: "uploads", Target: "/app/uploads"},
		{Type: "volume", Source: "local-data", Target: "/data"},
		{Type: "bind", Source: "/host", Target: "/container"},
	}

	resolved := ResolveTopLevelNFSVolumes(mounts, topLevel)

	// First: NFS named volume → converted to nfs type
	if resolved[0].Type != "nfs" {
		t.Errorf("expected nfs type, got %q", resolved[0].Type)
	}
	if resolved[0].Source != "nfs.internal:/exports/uploads" {
		t.Errorf("expected nfs source, got %q", resolved[0].Source)
	}
	if resolved[0].Tmpfs == nil || resolved[0].Tmpfs.Size != "vers=4,soft" {
		t.Errorf("expected NFS opts in Tmpfs.Size, got %+v", resolved[0].Tmpfs)
	}

	// Second: local named volume → unchanged
	if resolved[1].Type != "volume" {
		t.Errorf("local volume should be unchanged, got type %q", resolved[1].Type)
	}

	// Third: bind mount → unchanged
	if resolved[2].Type != "bind" {
		t.Errorf("bind mount should be unchanged, got type %q", resolved[2].Type)
	}
}

func TestResolveManifestVolumes(t *testing.T) {
	manifest := &types.BanyanManifest{
		Name: "test",
		Services: map[string]types.ManifestService{
			"api": {
				Image: "myapp",
				Volumes: types.VolumeMounts{
					{Type: "bind", Source: "./config", Target: "/etc/config"},
					{Type: "volume", Source: "shared", Target: "/data"},
				},
			},
		},
		Volumes: map[string]types.VolumeConfig{
			"shared": {
				Driver: "local",
				DriverOpts: map[string]string{
					"type":   "nfs",
					"o":      "addr=10.0.0.1,vers=4",
					"device": ":/exports/shared",
				},
			},
		},
	}

	types.ResolveManifestVolumes("", manifest)

	api := manifest.Services["api"]

	// Relative path NOT resolved on CLI (resolved on agent instead)
	if api.Volumes[0].Source != "./config" {
		t.Errorf("expected relative path unchanged, got %q", api.Volumes[0].Source)
	}

	// NFS named volume resolved
	if api.Volumes[1].Type != "nfs" {
		t.Errorf("expected nfs type, got %q", api.Volumes[1].Type)
	}
	if api.Volumes[1].Source != "10.0.0.1:/exports/shared" {
		t.Errorf("expected NFS source, got %q", api.Volumes[1].Source)
	}
}

func TestBuildNerdctlRunArgs_RelativeBindMount(t *testing.T) {
	origDir := bindMountDataDir
	bindMountDataDir = "/var/lib/banyan/data"
	t.Cleanup(func() { bindMountDataDir = origDir })

	task := &types.TaskRecord{
		ContainerName: "app-0",
		Image:         "myapp",
		Volumes: []types.VolumeMount{
			{Type: "bind", Source: "./config.yml", Target: "/etc/app/config.yml", ReadOnly: true},
		},
	}
	args := buildNerdctlRunArgs(task, false)
	argsStr := strings.Join(args, " ")

	expected := "-v /var/lib/banyan/data/config.yml:/etc/app/config.yml:ro"
	if !strings.Contains(argsStr, expected) {
		t.Errorf("expected relative path resolved on agent:\n  want: %s\n  got:  %s", expected, argsStr)
	}
}
