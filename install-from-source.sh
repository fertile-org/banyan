#!/usr/bin/env bash
set -euo pipefail

# Banyan installer (build from source)
#
# Builds banyan binaries from the current directory and installs dependencies.
# Requires Go 1.24+ on this machine.
#
# Usage:
#   sudo bash install-from-source.sh --role engine
#   sudo bash install-from-source.sh --role agent
#   sudo bash install-from-source.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared dependency functions
# shellcheck source=install-deps.sh
source "${SCRIPT_DIR}/install-deps.sh"

# --- Build from source ---

build_binary() {
    local name=$1
    local src_dir="${SCRIPT_DIR}/cmd/${name}"

    if [ ! -d "$src_dir" ]; then
        fatal "Source directory not found: ${src_dir}. Run this script from the banyan repo root."
    fi

    info "Building ${name} from source..."

    local tmp
    tmp=$(mktemp)

    if ! (cd "$src_dir" && GOWORK=off CGO_ENABLED=0 go build -o "$tmp" .); then
        rm -f "$tmp"
        fatal "Failed to build ${name}"
    fi

    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/${name}"

    info "${name} installed to ${INSTALL_DIR}/${name} (built from source)"
}

install_banyan() {
    if ! command -v go &>/dev/null; then
        fatal "Go is not installed. Install Go 1.24+ first: https://go.dev/dl/"
    fi

    build_binary "banyan-cli"

    if [ "$ROLE" = "engine" ] || [ "$ROLE" = "all" ]; then
        build_binary "banyan-engine"
    fi

    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        build_binary "banyan-agent"
    fi
}

# --- Main ---

main() {
    ROLE="all"

    while [ $# -gt 0 ]; do
        case "$1" in
            --role)
                ROLE="${2:-}"
                shift 2
                ;;
            --help|-h)
                echo "Banyan installer (build from source)"
                echo ""
                echo "Usage: install-from-source.sh [--role engine|agent|all]"
                echo ""
                echo "Options:"
                echo "  --role   Components to install: engine, agent, or all (default: all)"
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
    echo "  Banyan Installer (from source)"
    echo "========================================"
    echo ""

    if [ "$(id -u)" -ne 0 ]; then
        fatal "This script must be run as root. Use: sudo bash install-from-source.sh"
    fi

    detect_os
    detect_arch

    install_banyan
    install_deps
    install_systemd_services

    verify
}

main "$@"
