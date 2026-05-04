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

# Get latest version from GitHub
get_latest_version() {
    local version
    version=$(curl -sfL "${GITHUB_BASE}/releases/latest" 2>/dev/null | grep -oP 'tag/\K[^"]+' || true)
    if [ -z "$version" ]; then
        version=$(curl -sfL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/' || true)
    fi
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
    local tmp_file
    tmp_file=$(mktemp)

    if ! curl -sfL -o "$tmp_file" "$url"; then
        rm -f "$tmp_file"
        return 1
    fi

    echo "$tmp_file"
}

# Fallback: install via go install
go_install() {
    info "Attempting installation via 'go install'..."
    if ! command -v go &>/dev/null; then
        error "Go is not installed. Please install Go 1.21+ or download the binary manually from ${GITHUB_BASE}/releases"
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
