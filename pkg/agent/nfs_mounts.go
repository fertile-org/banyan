package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/types"
)

// nfsMountBase is the directory where NFS shares are mounted on the agent host.
var nfsMountBase = "/var/lib/banyan/nfs-mounts"

// mountCommand is the function used to run mount commands. Override in tests.
var mountCommand = defaultMountCommand

func defaultMountCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "mount", args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isMountedCommand checks if a path is a mount point. Override in tests.
var isMountedCommand = defaultIsMountedCommand

func defaultIsMountedCommand(path string) bool {
	cmd := exec.Command("mountpoint", "-q", path)
	return cmd.Run() == nil
}

// IsNFSVolume returns true if the volume has NFS driver_opts.
func IsNFSVolume(vol types.VolumeMount) bool {
	return vol.Type == "nfs"
}

// ResolveNFSVolumes resolves NFS-typed volumes in the list by mounting NFS on the host
// and converting them to bind mounts. Non-NFS volumes are returned unchanged.
func ResolveNFSVolumes(ctx context.Context, volumes []types.VolumeMount) ([]types.VolumeMount, error) {
	log := logging.New("agent.nfs")
	resolved := make([]types.VolumeMount, len(volumes))
	copy(resolved, volumes)

	for i, vol := range resolved {
		if !IsNFSVolume(vol) {
			continue
		}

		localPath, err := ensureNFSMount(ctx, vol, log)
		if err != nil {
			return nil, fmt.Errorf("NFS mount failed for %s: %w", vol.Target, err)
		}

		// Convert to bind mount so nerdctl can use it
		resolved[i] = types.VolumeMount{
			Type:     "bind",
			Source:   localPath,
			Target:   vol.Target,
			ReadOnly: vol.ReadOnly,
		}
	}
	return resolved, nil
}

// ensureNFSMount mounts an NFS share on the host if not already mounted.
// The volume's Source field contains "addr:device" and driver_opts-style options
// are encoded in the Type field as "nfs" with additional fields in Source.
//
// Expected format (set during volume resolution):
//
//	vol.Source = "addr:/device"  (NFS server address and export path)
//	vol.NFSOpts = "vers=4,soft" (mount options, optional)
func ensureNFSMount(ctx context.Context, vol types.VolumeMount, log *logging.Logger) (string, error) {
	// Parse source: "addr:/path" or just ":/path" with addr from opts
	source := vol.Source
	if source == "" {
		return "", fmt.Errorf("NFS volume source is empty")
	}

	// Compute a stable mount path from the source
	hash := sha256.Sum256([]byte(source))
	mountDir := filepath.Join(nfsMountBase, fmt.Sprintf("nfs-%x", hash[:8]))

	// Check if already mounted
	if isMountedCommand(mountDir) {
		log.Info("NFS share already mounted", "source", source, "path", mountDir)
		return mountDir, nil
	}

	// Create mount directory
	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		return "", fmt.Errorf("create NFS mount dir: %w", err)
	}

	// Mount: mount -t nfs [-o opts] addr:/path /local/path
	args := []string{"-t", "nfs"}
	if vol.Tmpfs != nil && vol.Tmpfs.Size != "" {
		// Reuse Tmpfs.Size field to carry NFS mount options (set during resolution)
		// This is a pragmatic reuse — NFS volumes don't use tmpfs options
		args = append(args, "-o", vol.Tmpfs.Size)
	}
	args = append(args, source, mountDir)

	log.Info("Mounting NFS share", "source", source, "target", mountDir)
	if err := mountCommand(ctx, args...); err != nil {
		return "", fmt.Errorf("mount -t nfs %s %s: %w", source, mountDir, err)
	}

	return mountDir, nil
}

// ResolveTopLevelNFSVolumes converts named volumes with NFS driver_opts into
// NFS-typed volume mounts that the agent can mount on the host.
// Called during manifest processing on the CLI/engine side.
func ResolveTopLevelNFSVolumes(mounts []types.VolumeMount, topLevel map[string]types.VolumeConfig) []types.VolumeMount {
	if len(topLevel) == 0 {
		return mounts
	}

	resolved := make([]types.VolumeMount, len(mounts))
	for i, m := range mounts {
		resolved[i] = m
		if m.Type != "volume" {
			continue
		}
		vc, ok := topLevel[m.Source]
		if !ok {
			continue
		}
		// Check if this is an NFS volume
		if vc.DriverOpts["type"] != "nfs" {
			continue
		}
		// Convert to NFS mount
		device := vc.DriverOpts["device"]
		opts := vc.DriverOpts["o"]

		// Parse addr from opts (e.g., "addr=nfs.internal,vers=4,soft")
		addr := ""
		var otherOpts []string
		for _, opt := range strings.Split(opts, ",") {
			opt = strings.TrimSpace(opt)
			if strings.HasPrefix(opt, "addr=") {
				addr = strings.TrimPrefix(opt, "addr=")
			} else if opt != "" {
				otherOpts = append(otherOpts, opt)
			}
		}

		if addr == "" || device == "" {
			continue // invalid NFS config, skip
		}

		nfsSource := addr + ":" + strings.TrimPrefix(device, ":")
		resolved[i] = types.VolumeMount{
			Type:     "nfs",
			Source:   nfsSource,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		}
		// Carry mount options via Tmpfs.Size (pragmatic reuse for NFS opts)
		if len(otherOpts) > 0 {
			resolved[i].Tmpfs = &types.TmpfsOpt{Size: strings.Join(otherOpts, ",")}
		}
	}
	return resolved
}
