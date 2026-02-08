#!/usr/bin/env bash
#
# Mote installation script
# Usage: curl -fsSL https://mote.dev/install.sh | bash
#

set -euo pipefail

# Configuration
REPO="mote-agent/mote"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="mote"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_banner() {
    echo -e "${BLUE}"
    echo "  __  __       _        "
    echo " |  \\/  | ___ | |_ ___  "
    echo " | |\\/| |/ _ \\| __/ _ \\ "
    echo " | |  | | (_) | ||  __/ "
    echo " |_|  |_|\\___/ \\__\\___| "
    echo ""
    echo -e "${NC}"
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Detect OS
detect_os() {
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "${os}" in
        linux*)  echo "linux" ;;
        darwin*) echo "darwin" ;;
        mingw*|msys*|cygwin*) echo "windows" ;;
        *)
            log_error "Unsupported operating system: ${os}"
            exit 1
            ;;
    esac
}

# Detect architecture
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "${arch}" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            log_error "Unsupported architecture: ${arch}"
            exit 1
            ;;
    esac
}

# Get latest version
get_latest_version() {
    local version
    version=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "${version}" ]; then
        log_error "Failed to get latest version"
        exit 1
    fi
    echo "${version}"
}

# Download and install
install_mote() {
    local os arch version download_url tmp_dir

    os="$(detect_os)"
    arch="$(detect_arch)"
    version="${VERSION:-$(get_latest_version)}"

    log_info "Detected: ${os}/${arch}"
    log_info "Installing version: ${version}"

    # Build download URL
    if [ "${os}" = "windows" ]; then
        download_url="https://github.com/${REPO}/releases/download/${version}/mote-${os}-${arch}.zip"
    else
        download_url="https://github.com/${REPO}/releases/download/${version}/mote-${os}-${arch}.tar.gz"
    fi

    log_info "Downloading from: ${download_url}"

    # Create temp directory
    tmp_dir=$(mktemp -d)
    trap "rm -rf ${tmp_dir}" EXIT

    # Download
    if command -v curl &> /dev/null; then
        curl -fsSL "${download_url}" -o "${tmp_dir}/mote.archive"
    elif command -v wget &> /dev/null; then
        wget -q "${download_url}" -O "${tmp_dir}/mote.archive"
    else
        log_error "Neither curl nor wget found"
        exit 1
    fi

    # Extract
    cd "${tmp_dir}"
    if [ "${os}" = "windows" ]; then
        unzip -q mote.archive
    else
        tar -xzf mote.archive
    fi

    # Find binary
    local binary
    binary=$(find . -name "mote*" -type f -perm -u+x 2>/dev/null | head -1)
    if [ -z "${binary}" ]; then
        binary=$(find . -name "mote*" -type f 2>/dev/null | head -1)
    fi

    if [ -z "${binary}" ]; then
        log_error "Binary not found in archive"
        exit 1
    fi

    # Install
    log_info "Installing to ${INSTALL_DIR}/${BINARY_NAME}"

    # Check if we need sudo
    if [ -w "${INSTALL_DIR}" ]; then
        mv "${binary}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        log_warn "Need sudo to install to ${INSTALL_DIR}"
        sudo mv "${binary}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    # Verify
    if "${INSTALL_DIR}/${BINARY_NAME}" version &> /dev/null; then
        log_info "Installation complete!"
        echo ""
        "${INSTALL_DIR}/${BINARY_NAME}" version
        echo ""
        log_info "Run 'mote auth login' to get started"
    else
        log_error "Installation verification failed"
        exit 1
    fi
}

# Main
main() {
    print_banner
    install_mote
}

main "$@"
