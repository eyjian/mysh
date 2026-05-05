#!/usr/bin/env bash
#
# mysh - MySQL CLI with syntax highlighting and auto-completion
# One-click install script
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
#   or
#   ./install.sh
#
set -euo pipefail

REPO="eyjian/mysh"
GITHUB_BASE="https://github.com/${REPO}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)       error "Unsupported OS: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Get latest version from GitHub API
get_latest_version() {
    local version
    # Use GitHub API with Accept header to get clean JSON
    version=$(curl -sfL -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name" *: *"([^"]+)".*/\1/' || true)
    echo "${version:-latest}"
}

# Download binary from GitHub Releases
download_binary() {
    local os="$1" arch="$2" version="$3"

    local ext=""
    if [ "$os" = "windows" ]; then
        ext=".exe"
    fi

    local filename="mysh-${os}-${arch}${ext}"
    local url="${GITHUB_BASE}/releases/download/${version}/${filename}"

    info "Downloading mysh ${version} for ${os}/${arch}..."
    info "URL: ${url}"
    local tmp_file
    tmp_file=$(mktemp)

    local http_code
    http_code=$(curl -sfL -o "$tmp_file" -w "%{http_code}" "$url" 2>/dev/null || true)

    if [ "$http_code" != "200" ]; then
        rm -f "$tmp_file"
        warn "Download failed (HTTP ${http_code:-unknown})"
        return 1
    fi

    # Verify it's a real binary (not an HTML error page)
    local file_type
    file_type=$(file "$tmp_file" 2>/dev/null || echo "unknown")
    if echo "$file_type" | grep -qi "html\|text"; then
        rm -f "$tmp_file"
        warn "Downloaded file is not a valid binary (got HTML/text)"
        return 1
    fi

    # Verify file size (> 1MB expected)
    local file_size
    file_size=$(stat -f%z "$tmp_file" 2>/dev/null || stat -c%s "$tmp_file" 2>/dev/null || echo "0")
    if [ "$file_size" -lt 1048576 ]; then
        rm -f "$tmp_file"
        warn "Downloaded file is too small (${file_size} bytes), likely not a valid binary"
        return 1
    fi

    echo "$tmp_file"
}

# Fallback: install via go install
go_install() {
    info "Attempting installation via 'go install'..."
    if ! command -v go &>/dev/null; then
        error "Go is not installed. Please install Go 1.24+ or download the binary manually from ${GITHUB_BASE}/releases"
    fi

    info "Running: go install github.com/${REPO}@latest"
    go install "github.com/${REPO}@latest"

    local go_bin
    go_bin="$(go env GOPATH)/bin/mysh"
    if [ -f "$go_bin" ]; then
        info "Installed to ${go_bin}"
        info "Make sure \$(go env GOPATH)/bin is in your PATH"
    fi
}

# Main install
main() {
    echo ""
    echo "  mysh - MySQL CLI with syntax highlighting & auto-completion"
    echo ""

    local os arch version
    os=$(detect_os)
    arch=$(detect_arch)
    version=$(get_latest_version)

    info "OS: ${os}  Arch: ${arch}  Version: ${version}"

    # Try GitHub Releases first
    local tmp_file
    if tmp_file=$(download_binary "$os" "$arch" "$version"); then
        chmod +x "$tmp_file"

        local target="${INSTALL_DIR}/mysh"
        if [ "$os" = "windows" ]; then
            target="${target}.exe"
        fi

        info "Installing to ${target}..."
        if [ -w "$(dirname "$target")" ]; then
            mv "$tmp_file" "$target"
        else
            sudo mv "$tmp_file" "$target" 2>/dev/null || {
                warn "Cannot write to ${INSTALL_DIR}, installing to ~/.local/bin instead"
                mkdir -p ~/.local/bin
                mv "$tmp_file" ~/.local/bin/mysh
                target=~/.local/bin/mysh
                warn "Please add ~/.local/bin to your PATH"
            }
        fi

        info "Successfully installed mysh to ${target}"
        info "Run 'mysh --help' to get started"
    else
        warn "Failed to download prebuilt binary, falling back to 'go install'"
        go_install
    fi
}

main "$@"
