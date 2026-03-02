#!/usr/bin/env bash
set -euo pipefail

# Banyan installer
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
#
# Options:
#   --role engine|agent|all   Components to install (default: all)
#   --version VERSION         Banyan version to install (default: latest)

REPO="fertile-org/banyan"
INSTALL_DIR="/usr/local/bin"

# Dependency versions
NERDCTL_VERSION="2.0.3"
CNI_VERSION="1.6.1"
ETCD_VERSION="3.5.17"
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

    # Resolve derivatives (e.g. Pop!_OS, Linux Mint, Zorin -> debian/ubuntu)
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
            # Fall back to ID_LIKE for unknown derivatives
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

# --- Install functions ---

get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    echo "$version"
}

install_binary() {
    local name=$1
    info "Installing ${name} ${BANYAN_VERSION}..."

    local url="https://github.com/${REPO}/releases/download/${BANYAN_VERSION}/${name}-linux-${ARCH}"
    local tmp
    tmp=$(mktemp)

    if ! curl -fsSL "$url" -o "$tmp"; then
        rm -f "$tmp"
        fatal "Failed to download ${name} from ${url}"
    fi

    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/${name}"

    info "${name} ${BANYAN_VERSION} installed to ${INSTALL_DIR}/${name}"
}

install_banyan() {
    if [ -z "$BANYAN_VERSION" ]; then
        info "Fetching latest version..."
        BANYAN_VERSION=$(get_latest_version)
        if [ -z "$BANYAN_VERSION" ]; then
            fatal "Could not determine latest version. Specify one with --version."
        fi
    fi

    # Always install banyan-cli
    install_binary "banyan-cli"

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        install_binary "banyan-engine"
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        install_binary "banyan-agent"
    fi
}

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
            # Download binary for non-Debian systems
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

    # Ensure containerd is running
    if systemctl is-active --quiet containerd 2>/dev/null; then
        info "containerd is running."
    else
        info "Starting containerd..."
        systemctl enable --now containerd
        info "containerd started."
    fi
}

install_nerdctl() {
    if command -v nerdctl &>/dev/null; then
        info "nerdctl already installed, skipping."
        return
    fi

    info "Installing nerdctl v${NERDCTL_VERSION}..."

    local url="https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${ARCH}.tar.gz"

    if ! curl -fsSL "$url" | tar -xz -C "${INSTALL_DIR}" nerdctl; then
        fatal "Failed to install nerdctl from ${url}"
    fi

    info "nerdctl installed."
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

    # Ensure buildkitd is running via systemd
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

    # Verify WireGuard kernel module is available (built-in since Linux 5.6)
    if ip link add wg-test type wireguard 2>/dev/null; then
        ip link delete wg-test 2>/dev/null
        info "WireGuard kernel support: OK"
    else
        error "WireGuard kernel module not available."
        error "  - WireGuard is required for overlay networking and the control tunnel."
        error "To enable WireGuard: modprobe wireguard (requires Linux 5.6+ or wireguard-dkms)"
        # Fatal for agents — WireGuard is now required (not optional)
        if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
            fatal "WireGuard is required for agents. Install the kernel module and try again."
        fi
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
        echo "    # Copy agent's public key to engine:"
        echo "    #   echo '<key>' > /etc/banyan/whitelisted-keys/<name>.pub"
        echo "    sudo systemctl enable --now banyan-agent   # start + enable on boot"
        echo ""
    fi
}

# --- Main ---

main() {
    ROLE="all"
    BANYAN_VERSION=""

    while [ $# -gt 0 ]; do
        case "$1" in
            --role)
                ROLE="${2:-}"
                shift 2
                ;;
            --version)
                BANYAN_VERSION="${2:-}"
                shift 2
                ;;
            --help|-h)
                echo "Banyan installer"
                echo ""
                echo "Usage: install.sh [--role engine|agent|all] [--version VERSION]"
                echo ""
                echo "Options:"
                echo "  --role      Components to install: engine, agent, or all (default: all)"
                echo "  --version   Banyan version to install (default: latest release)"
                exit 0
                ;;
            *)
                fatal "Unknown option: $1. Use --help for usage."
                ;;
        esac
    done

    case "$ROLE" in
        engine|agent|all) ;;
        *) fatal "Invalid role: $ROLE. Use: engine, agent, or all." ;;
    esac

    echo "========================================"
    echo "  Banyan Installer"
    echo "========================================"
    echo ""

    if [ "$(id -u)" -ne 0 ]; then
        fatal "This script must be run as root. Use: sudo bash install.sh"
    fi

    detect_os
    detect_arch

    install_banyan

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        install_etcd
        install_wireguard  # WireGuard is used for the control tunnel (encrypted gRPC)
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        install_containerd
        install_nerdctl
        install_cni
        install_wireguard  # WireGuard is used for both overlay networking and control tunnel
        install_buildkit
    fi

    install_systemd_services

    verify
}

main "$@"
