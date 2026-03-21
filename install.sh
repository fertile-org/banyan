#!/usr/bin/env bash
set -euo pipefail

# Banyan installer
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
#
# Security: Review this script before running. For manual installation, see the docs.
#   curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh -o install.sh
#   less install.sh
#   sudo bash install.sh
#
# Options:
#   --role engine|agent|all   Components to install (default: all)
#   --version VERSION         Banyan version to install (default: latest)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="fertile-org/banyan"

# Source shared dependency functions
# shellcheck source=install-deps.sh
source "${SCRIPT_DIR}/install-deps.sh"

# --- Release binary installation ---

get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    echo "$version"
}

verify_checksum() {
    local file=$1
    local expected=$2

    if [ -z "$expected" ]; then
        warn "No checksum available — skipping verification"
        return 0
    fi

    local actual
    actual=$(sha256sum "$file" | awk '{print $1}')
    if [ "$actual" != "$expected" ]; then
        return 1
    fi
    return 0
}

install_binary() {
    local name=$1
    info "Installing ${name} ${BANYAN_VERSION}..."

    local url="https://github.com/${REPO}/releases/download/${BANYAN_VERSION}/${name}-linux-${ARCH}"
    local checksum_url="https://github.com/${REPO}/releases/download/${BANYAN_VERSION}/checksums.txt"
    local tmp
    tmp=$(mktemp)

    if ! curl -fsSL "$url" -o "$tmp"; then
        rm -f "$tmp"
        fatal "Failed to download ${name} from ${url}"
    fi

    # Verify SHA-256 checksum if checksums.txt is published
    local checksums_tmp
    checksums_tmp=$(mktemp)
    if curl -fsSL "$checksum_url" -o "$checksums_tmp" 2>/dev/null; then
        local expected
        expected=$(grep "${name}-linux-${ARCH}" "$checksums_tmp" | awk '{print $1}')
        if [ -n "$expected" ]; then
            if ! verify_checksum "$tmp" "$expected"; then
                rm -f "$tmp" "$checksums_tmp"
                fatal "Checksum verification failed for ${name} — expected ${expected}"
            fi
            info "Checksum verified for ${name}"
        else
            warn "No checksum entry for ${name}-linux-${ARCH} in checksums.txt"
        fi
    else
        warn "checksums.txt not available — skipping verification"
    fi
    rm -f "$checksums_tmp"

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

    install_binary "banyan-cli"

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        install_binary "banyan-engine"
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        install_binary "banyan-agent"
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
    install_deps
    install_systemd_services

    verify
}

main "$@"
