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
    OS_VERSION="${VERSION_ID:-unknown}"

    case "$OS" in
        ubuntu|debian)
            PKG_UPDATE="apt-get update -qq"
            PKG_INSTALL="apt-get install -y -qq"
            ;;
        centos|rhel|fedora|rocky|almalinux)
            if command -v dnf &>/dev/null; then
                PKG_UPDATE="true"
                PKG_INSTALL="dnf install -y -q"
            else
                PKG_UPDATE="true"
                PKG_INSTALL="yum install -y -q"
            fi
            ;;
        *)
            fatal "Unsupported OS: $OS. Supported: Ubuntu, Debian, CentOS, RHEL, Fedora, Rocky, AlmaLinux."
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

install_banyan() {
    if [ -z "$VERSION" ]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [ -z "$VERSION" ]; then
            fatal "Could not determine latest version. Specify one with --version."
        fi
    fi

    info "Installing banyan-cli ${VERSION}..."

    local url="https://github.com/${REPO}/releases/download/${VERSION}/banyan-cli-linux-${ARCH}"
    local tmp
    tmp=$(mktemp)

    if ! curl -fsSL "$url" -o "$tmp"; then
        rm -f "$tmp"
        fatal "Failed to download banyan-cli from ${url}"
    fi

    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/banyan-cli"

    info "banyan-cli ${VERSION} installed to ${INSTALL_DIR}/banyan-cli"
}

install_etcd() {
    if command -v etcd &>/dev/null; then
        info "etcd already installed, skipping."
        return
    fi

    info "Installing etcd..."

    case "$OS" in
        ubuntu|debian)
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
            ubuntu|debian)
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
        if command -v etcd &>/dev/null; then
            info "  etcd: OK"
        else
            error "  etcd: NOT FOUND"
            ok=false
        fi
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
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
        echo "  Start the Engine:"
        echo "    sudo banyan-cli engine init"
        echo "    sudo banyan-cli engine start"
        echo ""
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        echo "  Start an Agent:"
        echo "    sudo banyan-cli agent init"
        echo "    sudo banyan-cli agent start --engine http://<engine-ip>:2379"
        echo ""
    fi
}

# --- Main ---

main() {
    ROLE="all"
    VERSION=""

    while [ $# -gt 0 ]; do
        case "$1" in
            --role)
                ROLE="${2:-}"
                shift 2
                ;;
            --version)
                VERSION="${2:-}"
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
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        install_containerd
        install_nerdctl
        install_cni
    fi

    verify
}

main "$@"
