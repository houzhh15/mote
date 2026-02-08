#!/usr/bin/env bash
#
# Build release binaries for all supported platforms
#

set -euo pipefail

# Configuration
DIST_DIR="${DIST_DIR:-dist}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

LDFLAGS="-s -w -X mote/internal/version.Version=${VERSION} -X mote/internal/version.GitCommit=${GIT_COMMIT} -X mote/internal/version.BuildTime=${BUILD_TIME}"

# Platforms to build
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Create dist directory
mkdir -p "${DIST_DIR}"
log_info "Building version ${VERSION} (${GIT_COMMIT})"

# Build for each platform
for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "${platform}"
    
    output_name="mote-${GOOS}-${GOARCH}"
    if [ "${GOOS}" = "windows" ]; then
        output_name="${output_name}.exe"
    fi
    
    output_path="${DIST_DIR}/${output_name}"
    
    log_info "Building ${GOOS}/${GOARCH}..."
    
    if CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="${LDFLAGS}" -o "${output_path}" ./cmd/mote; then
        log_info "  -> ${output_path}"
        
        # Compress
        cd "${DIST_DIR}"
        if [ "${GOOS}" = "windows" ]; then
            zip -q "${output_name%.exe}.zip" "${output_name}"
            log_info "  -> ${output_name%.exe}.zip"
        else
            tar -czf "${output_name}.tar.gz" "${output_name}"
            log_info "  -> ${output_name}.tar.gz"
        fi
        cd - > /dev/null
    else
        log_error "Failed to build ${GOOS}/${GOARCH}"
        exit 1
    fi
done

# Generate checksums
log_info "Generating checksums..."
cd "${DIST_DIR}"
if command -v sha256sum &> /dev/null; then
    sha256sum *.tar.gz *.zip 2>/dev/null > checksums.txt || true
elif command -v shasum &> /dev/null; then
    shasum -a 256 *.tar.gz *.zip 2>/dev/null > checksums.txt || true
else
    log_warn "sha256sum/shasum not found, skipping checksums"
fi
cd - > /dev/null

log_info "Build complete! Files are in ${DIST_DIR}/"
ls -la "${DIST_DIR}/"
