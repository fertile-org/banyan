package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	cniPluginsVersion = "v1.8.0"
	flannelVersion    = "v1.7.1-flannel1"
	flanneldVersion   = "v0.25.4"
	cniBinDir         = "/opt/cni/bin"
	flanneldBinDir    = "/usr/local/bin"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup CNI plugins and dependencies",
	Long:  "Downloads and installs required CNI plugins for Banyan VPC networking",
}

var setupCNICmd = &cobra.Command{
	Use:   "cni",
	Short: "Install CNI plugins (flannel, bridge, etc.)",
	Long: `Automatically downloads and installs CNI plugins required for Banyan networking.

This includes:
- Standard CNI plugins (bridge, host-local, portmap, etc.)
- Flannel CNI plugin for VXLAN overlay networking

Installation directory: /opt/cni/bin (requires sudo)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🔧 Setting up CNI plugins for Banyan VPC...")
		fmt.Println()

		// Check if running as root
		if os.Geteuid() != 0 {
			return fmt.Errorf("this command requires root privileges, please run with sudo")
		}

		// Detect architecture
		arch := detectArchitecture()
		fmt.Printf("✓ Detected architecture: %s\n", arch)

		// Check what's already installed
		cniInstalled := false
		flanneldInstalled := false

		if _, err := os.Stat(cniBinDir + "/flannel"); err == nil {
			fmt.Println("✓ CNI plugins already installed")
			cniInstalled = true
		}

		if _, err := os.Stat(flanneldBinDir + "/flanneld"); err == nil {
			fmt.Println("✓ Flannel daemon already installed")
			flanneldInstalled = true
		}

		if cniInstalled && flanneldInstalled {
			fmt.Println("\n✅ All components already installed!")
			fmt.Println("\nYou can now use:")
			fmt.Println("  banyan-cli cni setup-plugin flannel")
			fmt.Println("  banyan-cli cni add-container <container-id> <network-id> <ip>")
			return nil
		}

		// Install CNI plugins if needed
		if !cniInstalled {
			// Create CNI bin directory
			fmt.Printf("Creating directory: %s\n", cniBinDir)
			if err := os.MkdirAll(cniBinDir, 0755); err != nil {
				return fmt.Errorf("failed to create %s: %w", cniBinDir, err)
			}

			// Install standard CNI plugins
			if err := installStandardCNIPlugins(arch); err != nil {
				return fmt.Errorf("failed to install standard CNI plugins: %w", err)
			}

			// Install Flannel CNI plugin
			if err := installFlannelCNIPlugin(arch); err != nil {
				return fmt.Errorf("failed to install Flannel CNI plugin: %w", err)
			}
		}

		// Install Flannel daemon if needed
		if !flanneldInstalled {
			if err := installFlannelDaemon(arch); err != nil {
				return fmt.Errorf("failed to install Flannel daemon: %w", err)
			}
		}

		// Verify installation
		fmt.Println("\n🔍 Verifying installation...")
		if err := verifyCNIInstallation(); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		// Verify flanneld
		if _, err := os.Stat(flanneldBinDir + "/flanneld"); os.IsNotExist(err) {
			return fmt.Errorf("flanneld verification failed: binary not found at %s", flanneldBinDir+"/flanneld")
		}
		fmt.Printf("✓ Found: flanneld\n")

		fmt.Println("\n✅ CNI plugins and Flannel daemon installed successfully!")
		fmt.Println("\nYou can now use:")
		fmt.Println("  banyan-cli cni setup-plugin flannel")
		fmt.Println("  banyan-cli cni add-container <container-id> <network-id> <ip>")

		return nil
	},
}

func detectArchitecture() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	case "386":
		return "386"
	case "ppc64le":
		return "ppc64le"
	case "s390x":
		return "s390x"
	default:
		fmt.Printf("Warning: unsupported architecture %s, defaulting to amd64\n", arch)
		return "amd64"
	}
}

func installStandardCNIPlugins(arch string) error {
	fmt.Println("\n📦 Installing standard CNI plugins...")

	url := fmt.Sprintf(
		"https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-linux-%s-%s.tgz",
		cniPluginsVersion, arch, cniPluginsVersion,
	)

	fmt.Printf("Downloading from: %s\n", url)

	// Download to temporary file
	tmpFile := "/tmp/cni-plugins.tgz"
	if err := downloadFile(url, tmpFile); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile)

	fmt.Println("Extracting plugins...")

	// Extract to /opt/cni/bin
	cmd := exec.Command("tar", "-C", cniBinDir, "-xzf", tmpFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	fmt.Println("✓ Standard CNI plugins installed")
	fmt.Println("  - bridge")
	fmt.Println("  - host-local (IPAM)")
	fmt.Println("  - portmap")
	fmt.Println("  - vlan")
	fmt.Println("  - and more...")

	return nil
}

func installFlannelCNIPlugin(arch string) error {
	fmt.Println("\n📦 Installing Flannel CNI plugin...")

	url := fmt.Sprintf(
		"https://github.com/flannel-io/cni-plugin/releases/download/%s/flannel-%s",
		flannelVersion, arch,
	)

	fmt.Printf("Downloading from: %s\n", url)

	flannelBin := cniBinDir + "/flannel"
	if err := downloadFile(url, flannelBin); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Make executable
	if err := os.Chmod(flannelBin, 0755); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	fmt.Println("✓ Flannel CNI plugin installed")

	return nil
}

func installFlannelDaemon(arch string) error {
	fmt.Println("\n📦 Installing Flannel daemon (flanneld)...")

	url := fmt.Sprintf(
		"https://github.com/flannel-io/flannel/releases/download/%s/flannel-%s-linux-%s.tar.gz",
		flanneldVersion, flanneldVersion, arch,
	)

	fmt.Printf("Downloading from: %s\n", url)

	// Download to temporary file
	tmpFile := "/tmp/flanneld.tar.gz"
	if err := downloadFile(url, tmpFile); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile)

	fmt.Println("Extracting flanneld binary...")

	// Extract to temporary directory
	tmpDir := "/tmp/flannel-extract"
	os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("tar", "-C", tmpDir, "-xzf", tmpFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Move flanneld to /usr/local/bin
	flanneldSrc := filepath.Join(tmpDir, "flanneld")
	flanneldDst := filepath.Join(flanneldBinDir, "flanneld")

	data, err := os.ReadFile(flanneldSrc)
	if err != nil {
		return fmt.Errorf("failed to read flanneld: %w", err)
	}

	if err := os.WriteFile(flanneldDst, data, 0755); err != nil {
		return fmt.Errorf("failed to install flanneld: %w", err)
	}

	fmt.Println("✓ Flannel daemon installed at /usr/local/bin/flanneld")

	return nil
}

func downloadFile(url, filepath string) error {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func verifyCNIInstallation() error {
	requiredPlugins := []string{
		"flannel",
		"bridge",
		"host-local",
		"portmap",
	}

	allFound := true
	for _, plugin := range requiredPlugins {
		path := cniBinDir + "/" + plugin
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("✗ Missing: %s\n", plugin)
			allFound = false
		} else {
			fmt.Printf("✓ Found: %s\n", plugin)
		}
	}

	if !allFound {
		return fmt.Errorf("some required plugins are missing")
	}

	return nil
}

func init() {
	setupCmd.AddCommand(setupCNICmd)
	rootCmd.AddCommand(setupCmd)
}
