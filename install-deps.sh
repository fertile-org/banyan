#!/usr/bin/env bash
# Shared dependency installation functions for Banyan.
# Sourced by both install.sh (release binaries) and install-from-source.sh (build from source).
#
# This file is NOT meant to be run directly.

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Dependency versions
NERDCTL_VERSION="2.1.3"
NERDCTL_MIN_VERSION="2.1.3"  # minimum for --health-cmd support
CNI_VERSION="1.6.2"
ETCD_VERSION="3.5.17"
REGISTRY_VERSION="2.8.3"
BUILDKIT_VERSION="0.19.0"

# --- Output helpers ---

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
fatal() { error "$*"; exit 1; }

# --- Detection ---

detect_os() {
    if [ ! -f /etc/os-release ]; then
        fatal "Cannot detect OS. /etc/os-release not found."
    fi

    . /etc/os-release
    OS="$ID"
    OS_LIKE="${ID_LIKE:-}"
    OS_VERSION="${VERSION_ID:-unknown}"

    case "$OS" in
        ubuntu|debian|pop|linuxmint|zorin|elementary|neon)
            PKG_UPDATE="apt-get update -qq"
            PKG_INSTALL="apt-get install -y -qq"
            ;;
        centos|rhel|fedora|rocky|almalinux|ol)
            if command -v dnf &>/dev/null; then
                PKG_UPDATE="true"
                PKG_INSTALL="dnf install -y -q"
            else
                PKG_UPDATE="true"
                PKG_INSTALL="yum install -y -q"
            fi
            ;;
        *)
            if echo "$OS_LIKE" | grep -qw "debian\|ubuntu"; then
                PKG_UPDATE="apt-get update -qq"
                PKG_INSTALL="apt-get install -y -qq"
            elif echo "$OS_LIKE" | grep -qw "rhel\|fedora\|centos"; then
                if command -v dnf &>/dev/null; then
                    PKG_UPDATE="true"
                    PKG_INSTALL="dnf install -y -q"
                else
                    PKG_UPDATE="true"
                    PKG_INSTALL="yum install -y -q"
                fi
            else
                fatal "Unsupported OS: $OS. Supported: Debian/Ubuntu-based and RHEL/Fedora-based distributions."
            fi
            ;;
    esac

    info "Detected OS: $OS $OS_VERSION"
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        *)       fatal "Unsupported architecture: $(uname -m). Supported: x86_64, aarch64." ;;
    esac
    info "Detected architecture: $ARCH"
}

# --- Dependency install functions ---

install_etcd() {
    if command -v etcd &>/dev/null; then
        info "etcd already installed, skipping."
        return
    fi

    info "Installing etcd..."

    case "$OS" in
        ubuntu|debian|pop|linuxmint|zorin|elementary|neon)
            $PKG_UPDATE
            $PKG_INSTALL etcd-server
            ;;
        *)
            info "Downloading etcd v${ETCD_VERSION} binary..."
            local url="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-${ARCH}.tar.gz"
            local tmp
            tmp=$(mktemp -d)
            if ! curl -fsSL "$url" | tar -xz -C "$tmp" --strip-components=1; then
                rm -rf "$tmp"
                fatal "Failed to download etcd from ${url}"
            fi
            mv "$tmp/etcd" "$tmp/etcdctl" "${INSTALL_DIR}/"
            rm -rf "$tmp"
            ;;
    esac

    info "etcd installed."
}

install_registry() {
    if command -v registry &>/dev/null; then
        info "Distribution registry already installed, skipping."
        return
    fi

    info "Installing Distribution registry v${REGISTRY_VERSION}..."

    local url="https://github.com/distribution/distribution/releases/download/v${REGISTRY_VERSION}/registry_${REGISTRY_VERSION}_linux_${ARCH}.tar.gz"
    local tmp
    tmp=$(mktemp -d)
    if ! curl -fsSL "$url" | tar -xz -C "$tmp"; then
        rm -rf "$tmp"
        fatal "Failed to download Distribution registry from ${url}"
    fi
    mv "$tmp/registry" "${INSTALL_DIR}/"
    chmod +x "${INSTALL_DIR}/registry"
    rm -rf "$tmp"

    info "Distribution registry v${REGISTRY_VERSION} installed."
}

install_containerd() {
    if command -v containerd &>/dev/null; then
        info "containerd already installed, skipping."
    else
        info "Installing containerd..."

        case "$OS" in
            ubuntu|debian|pop|linuxmint|zorin|elementary|neon)
                $PKG_UPDATE
                $PKG_INSTALL containerd
                ;;
            *)
                $PKG_INSTALL containerd.io 2>/dev/null || $PKG_INSTALL containerd
                ;;
        esac
    fi

    if systemctl is-active --quiet containerd 2>/dev/null; then
        info "containerd is running."
    else
        info "Starting containerd..."
        systemctl enable --now containerd
        info "containerd started."
    fi
}

version_ge() {
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

install_nerdctl() {
    if command -v nerdctl &>/dev/null; then
        local current
        current=$(nerdctl --version 2>/dev/null | grep -oP '\d+\.\d+\.\d+' | head -1)
        if [ -n "$current" ] && version_ge "$current" "$NERDCTL_MIN_VERSION"; then
            info "nerdctl v${current} already installed (>= ${NERDCTL_MIN_VERSION}), skipping."
            return
        fi
        warn "nerdctl v${current} is below minimum v${NERDCTL_MIN_VERSION} (required for healthcheck support). Upgrading..."
    fi

    info "Installing nerdctl v${NERDCTL_VERSION}..."

    local url="https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${ARCH}.tar.gz"

    if ! curl -fsSL "$url" | tar -xz -C "${INSTALL_DIR}" nerdctl; then
        fatal "Failed to install nerdctl from ${url}"
    fi

    info "nerdctl v${NERDCTL_VERSION} installed."
}

install_cni() {
    local cni_dir="/opt/cni/bin"

    if [ -d "$cni_dir" ] && [ -n "$(ls -A "$cni_dir" 2>/dev/null)" ]; then
        info "CNI plugins already installed, skipping."
        return
    fi

    info "Installing CNI plugins v${CNI_VERSION}..."

    mkdir -p "$cni_dir"
    local url="https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-${ARCH}-v${CNI_VERSION}.tgz"

    if ! curl -fsSL "$url" | tar -xz -C "$cni_dir"; then
        fatal "Failed to install CNI plugins from ${url}"
    fi

    info "CNI plugins installed."
}

install_wireguard() {
    if command -v wg &>/dev/null; then
        info "wireguard-tools already installed, skipping."
    else
        info "Installing wireguard-tools..."

        case "$OS" in
            ubuntu|debian|pop|linuxmint|zorin|elementary|neon)
                $PKG_UPDATE
                $PKG_INSTALL wireguard-tools
                ;;
            *)
                $PKG_INSTALL wireguard-tools
                ;;
        esac

        info "wireguard-tools installed."
    fi

    if ip link add wg-test type wireguard 2>/dev/null; then
        ip link delete wg-test 2>/dev/null
        info "WireGuard kernel support: OK"
    else
        error "WireGuard kernel module not available."
        error "  - WireGuard is required for overlay networking and the control tunnel."
        error "To enable WireGuard: modprobe wireguard (requires Linux 5.6+ or wireguard-dkms)"
        if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
            fatal "WireGuard is required for agents. Install the kernel module and try again."
        fi
    fi
}

install_buildkit() {
    if command -v buildkitd &>/dev/null; then
        info "BuildKit already installed, skipping."
    else
        info "Installing BuildKit v${BUILDKIT_VERSION}..."

        local url="https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/buildkit-v${BUILDKIT_VERSION}.linux-${ARCH}.tar.gz"
        local tmp
        tmp=$(mktemp -d)
        if ! curl -fsSL "$url" | tar -xz -C "$tmp"; then
            rm -rf "$tmp"
            fatal "Failed to download BuildKit from ${url}"
        fi
        mv "$tmp/bin/buildkitd" "$tmp/bin/buildctl" "${INSTALL_DIR}/"
        rm -rf "$tmp"

        info "BuildKit installed."
    fi

    if ! systemctl is-active --quiet buildkit 2>/dev/null; then
        info "Setting up buildkitd service..."
        cat > /etc/systemd/system/buildkit.service <<'UNIT'
[Unit]
Description=BuildKit
After=containerd.service

[Service]
ExecStart=/usr/local/bin/buildkitd --oci-worker=false --containerd-worker=true
Restart=always

[Install]
WantedBy=multi-user.target
UNIT
        systemctl daemon-reload
        systemctl enable --now buildkit
        info "buildkitd started."
    else
        info "buildkitd is running."
    fi
}

# --- Systemd services ---

install_systemd_services() {
    info "Installing systemd service files..."

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        cat > /etc/systemd/system/banyan-engine.service <<EOF
[Unit]
Description=Banyan Engine
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/banyan-engine start
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
        info "  banyan-engine.service installed"
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        cat > /etc/systemd/system/banyan-agent.service <<EOF
[Unit]
Description=Banyan Agent
After=network-online.target containerd.service
Wants=network-online.target containerd.service

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/banyan-agent start
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
        info "  banyan-agent.service installed"
    fi

    systemctl daemon-reload
    info "systemd services ready (not yet enabled — run init first)"
}

# --- Verify ---

verify() {
    echo ""
    info "Verifying installation..."

    local ok=true

    if command -v banyan-cli &>/dev/null; then
        info "  banyan-cli: OK"
    else
        error "  banyan-cli: NOT FOUND"
        ok=false
    fi

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        if command -v banyan-engine &>/dev/null; then
            info "  banyan-engine: OK"
        else
            error "  banyan-engine: NOT FOUND"
            ok=false
        fi

        if command -v etcd &>/dev/null; then
            info "  etcd: OK"
        else
            error "  etcd: NOT FOUND"
            ok=false
        fi

        if command -v registry &>/dev/null; then
            info "  registry: OK"
        else
            error "  registry: NOT FOUND"
            ok=false
        fi

        if command -v wg &>/dev/null; then
            info "  wireguard-tools: OK (for control tunnel)"
        else
            error "  wireguard-tools: NOT FOUND (required for control tunnel)"
            ok=false
        fi
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        if command -v banyan-agent &>/dev/null; then
            info "  banyan-agent: OK"
        else
            error "  banyan-agent: NOT FOUND"
            ok=false
        fi

        if command -v containerd &>/dev/null; then
            info "  containerd: OK"
        else
            error "  containerd: NOT FOUND"
            ok=false
        fi

        if command -v nerdctl &>/dev/null; then
            info "  nerdctl: OK"
        else
            error "  nerdctl: NOT FOUND"
            ok=false
        fi

        if command -v buildkitd &>/dev/null; then
            info "  buildkit: OK"
        else
            error "  buildkit: NOT FOUND"
            ok=false
        fi

        if command -v wg &>/dev/null; then
            info "  wireguard-tools: OK"
        else
            error "  wireguard-tools: NOT FOUND (required for overlay networking and control tunnel)"
            ok=false
        fi

    fi

    if ! $ok; then
        fatal "Some components failed to install. Check the errors above."
    fi

    echo ""
    echo "========================================"
    echo "  Installation complete!"
    echo "========================================"
    echo ""

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        echo "  Engine:"
        echo "    sudo banyan-engine init                    # one-time setup"
        echo "    sudo systemctl enable --now banyan-engine  # start + enable on boot"
        echo ""
        echo "  Note: Engine init displays a public key — share it with agents/CLI"
        echo "  to enable encrypted control tunnels (port 51821/UDP)."
        echo ""
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        echo "  Agent:"
        echo "    sudo banyan-agent init                     # one-time setup"
        echo "    # Whitelist agent on the engine:"
        echo "    #   sudo banyan-engine add-client --name <name> --pubkey <key>"
        echo "    sudo systemctl enable --now banyan-agent   # start + enable on boot"
        echo ""
    fi
}

# --- Install dependencies (called by both install scripts) ---

install_nfs_client() {
    if command -v mount.nfs &>/dev/null; then
        info "NFS client already installed, skipping."
        return
    fi

    info "Installing NFS client tools..."

    case "$OS" in
        ubuntu|debian|pop|linuxmint|zorin|elementary|neon)
            $PKG_UPDATE
            $PKG_INSTALL nfs-common
            ;;
        *)
            $PKG_INSTALL nfs-utils
            ;;
    esac

    info "NFS client installed."
}

install_deps() {
    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        install_etcd
        install_registry
        install_wireguard
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        install_containerd
        install_nerdctl
        install_cni
        install_wireguard
        install_buildkit
        install_nfs_client  # For NFS volume mounts
    fi
}
