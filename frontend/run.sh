#!/bin/sh

# Bootstrapping launcher for aist.
#
# Checks the installed version against the latest release, upgrades when
# outdated, then executes aist. It is served by the aist server, which injects
# its own URL so the client is downloaded from, and logs in to, that server:
#
#   curl -fsSL https://<server>/run.sh | sh
#   wget -qO- https://<server>/run.sh | sh
#
# Release layout:
#   {AIST_RELEASE_BASE_URL}/<version>/aist-<version>-<os>-<arch>.tar.gz
#   {AIST_RELEASE_BASE_URL}/<version>/checksums.txt
#   {AIST_RELEASE_BASE_URL}/latest.txt
#   {AIST_RELEASE_BASE_URL}/latest.json
#
# Environment:
#   AIST_SERVER            server to download from and log in to (defaults to
#                          the server that served this script)
#   AIST_RELEASE_BASE_URL  artifact root (defaults to {AIST_SERVER}/releases/client)

set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

BINARY_NAME="aist"
DEFAULT_SERVER_URL="__AIST_SERVER_URL__"
case "$DEFAULT_SERVER_URL" in
    __*) DEFAULT_SERVER_URL="https://aistrun.cc" ;;
esac
SERVER_URL="${AIST_SERVER:-$DEFAULT_SERVER_URL}"
SERVER_URL="${SERVER_URL%/}"
AIST_RELEASE_BASE_URL="${AIST_RELEASE_BASE_URL:-${SERVER_URL}/releases/client}"

print_info() {
    printf '%b\n' "${BLUE}ℹ${NC} $1" >&2
}

print_success() {
    printf '%b\n' "${GREEN}✓${NC} $1" >&2
}

print_warning() {
    printf '%b\n' "${YELLOW}⚠${NC} $1" >&2
}

print_error() {
    printf '%b\n' "${RED}✗${NC} $1" >&2
}

fetch_with_retry() {
    local mode="$1"
    local url="$2"
    local output="$3"
    local attempt=1
    local max_attempts=3

    while [ "$attempt" -le "$max_attempts" ]; do
        rm -f "$output"
        if command -v curl >/dev/null 2>&1; then
            if [ "$mode" = "progress" ]; then
                if curl -fL -# "$url" -o "$output"; then
                    return 0
                fi
            else
                if curl -fsSL "$url" -o "$output"; then
                    return 0
                fi
            fi
        elif command -v wget >/dev/null 2>&1; then
            if [ "$mode" = "progress" ]; then
                if wget --help 2>&1 | grep -q -- '--show-progress'; then
                    if wget -q --show-progress -O "$output" "$url"; then
                        return 0
                    fi
                else
                    if wget -O "$output" "$url"; then
                        return 0
                    fi
                fi
            else
                if wget -q -O "$output" "$url"; then
                    return 0
                fi
            fi
        else
            print_error "Neither curl nor wget found. Please install one of them."
            return 1
        fi

        if [ "$attempt" -lt "$max_attempts" ]; then
            sleep 1
        fi
        attempt=$((attempt + 1))
    done

    return 1
}

fetch_url() {
    fetch_with_retry "quiet" "$1" "$2"
}

fetch_url_progress() {
    fetch_with_retry "progress" "$1" "$2"
}

# Return a sanitized version string, or fail if it is not a valid tag.
parse_version() {
    local version
    version=$(printf '%s' "$1" | tr -d '\r' | awk 'NF { print $1; exit }')
    case "$version" in
        v[0-9]*) ;;
        *) return 1 ;;
    esac
    case "$version" in
        *[!A-Za-z0-9._-]*) return 1 ;;
    esac
    printf '%s\n' "$version"
}

# Read the latest version from {base}/latest.txt, falling back to latest.json.
resolve_latest_version() {
    local base="${AIST_RELEASE_BASE_URL%/}"
    local tmp version
    tmp=$(mktemp)

    if fetch_url "${base}/latest.txt" "$tmp" 2>/dev/null; then
        if version=$(parse_version "$(cat "$tmp")"); then
            rm -f "$tmp"
            printf '%s\n' "$version"
            return 0
        fi
    fi

    if fetch_url "${base}/latest.json" "$tmp" 2>/dev/null; then
        version=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp" | head -n 1)
        if version=$(parse_version "$version"); then
            rm -f "$tmp"
            printf '%s\n' "$version"
            return 0
        fi
    fi

    rm -f "$tmp"
    return 1
}

detect_os() {
    case "$(uname -s)" in
        Linux*) echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)
            print_error "Unsupported operating system: $(uname -s)"
            return 1
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv6*|armv7*|armv8*|armhf|arm) echo "arm" ;;
        i386|i486|i586|i686|x86) echo "386" ;;
        riscv64) echo "riscv64" ;;
        *)
            print_error "Unsupported architecture: $(uname -m)"
            return 1
            ;;
    esac
}

validate_platform() {
    local os="$1"
    local arch="$2"

    case "${os}/${arch}" in
        linux/amd64|linux/arm64|linux/arm|linux/386|linux/riscv64|darwin/amd64|darwin/arm64)
            return 0
            ;;
        *)
            print_error "Unsupported platform: ${os}/${arch}"
            return 1
            ;;
    esac
}

download_archive() {
    local version="$1"
    local os="$2"
    local arch="$3"
    local file_name="${BINARY_NAME}-${version}-${os}-${arch}.tar.gz"
    local download_url="${AIST_RELEASE_BASE_URL%/}/${version}/${file_name}"
    local tmp_dir
    tmp_dir=$(mktemp -d)
    local target_file="${tmp_dir}/${file_name}"

    print_info "Downloading ${BINARY_NAME} ${version} for ${os}/${arch}..."
    print_info "URL: ${download_url}"

    if ! fetch_url_progress "$download_url" "$target_file"; then
        print_error "Failed to download ${file_name}"
        rm -rf "$tmp_dir"
        return 1
    fi

    print_success "Download completed"
    echo "$target_file"
}

# Extract the binary from a verified archive. Prints the binary path.
extract_binary() {
    local archive="$1"
    local version="$2"
    local os="$3"
    local arch="$4"
    local binary_name="${BINARY_NAME}-${version}-${os}-${arch}"
    local dir
    dir=$(dirname "$archive")

    if ! command -v tar >/dev/null 2>&1; then
        print_error "tar is required to extract ${BINARY_NAME}."
        return 1
    fi
    if ! tar -xzf "$archive" -C "$dir" "$binary_name"; then
        print_error "Failed to extract ${binary_name} from $(basename "$archive")"
        return 1
    fi
    if [ ! -f "${dir}/${binary_name}" ]; then
        print_error "Archive does not contain ${binary_name}"
        return 1
    fi
    echo "${dir}/${binary_name}"
}

# Verify the downloaded archive against checksums.txt. Fails closed: any missing
# tool, missing entry or mismatch aborts the install.
verify_checksum() {
    local target_file="$1"
    local version="$2"
    local file_name
    file_name=$(basename "$target_file")
    local checksums_url="${AIST_RELEASE_BASE_URL%/}/${version}/checksums.txt"
    local checksums_file="${target_file}.checksums"

    print_info "Verifying checksum for ${file_name}..."
    if ! fetch_url "$checksums_url" "$checksums_file"; then
        print_error "Failed to download checksums.txt (${checksums_url})"
        return 1
    fi

    local expected
    expected=$(awk -v f="$file_name" '$2 == f { print $1 }' "$checksums_file")
    if [ -z "$expected" ]; then
        print_error "No checksum entry for ${file_name} in checksums.txt"
        return 1
    fi

    local actual=""
    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$target_file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$target_file" | awk '{print $1}')
    else
        print_error "No sha256 tool found (need sha256sum or shasum)"
        return 1
    fi

    if [ "$expected" != "$actual" ]; then
        print_error "Checksum mismatch for ${file_name}: expected ${expected}, got ${actual}"
        return 1
    fi

    print_success "Checksum verified"
    return 0
}

# Resolve the running aist binary on PATH.
existing_install_path() {
    local path
    path=$(command -v "$BINARY_NAME" 2>/dev/null) || return 1
    [ -n "$path" ] || return 1

    while [ -L "$path" ]; do
        local link
        link=$(readlink "$path")
        case "$link" in
            /*) path="$link" ;;
            *) path="$(dirname "$path")/$link" ;;
        esac
    done

    [ -f "$path" ] && [ -x "$path" ] || return 1
    printf '%s\n' "$path"
}

# Locate an existing aist installation on PATH and return its directory.
existing_install_dir() {
    local path dir
    path=$(existing_install_path) || return 1
    dir=$(cd "$(dirname "$path")" 2>/dev/null && pwd) || return 1
    [ -w "$dir" ] || return 1
    echo "$dir"
    return 0
}

resolve_install_dir() {
    local dir

    if dir=$(existing_install_dir); then
        echo "$dir"
        return 0
    fi

    for dir in "/usr/local/bin" "${HOME}/.local/bin" "${HOME}/bin"; do
        if [ -d "$dir" ] && [ -w "$dir" ]; then
            echo "$dir"
            return 0
        fi
        if [ ! -e "$dir" ] && mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; then
            echo "$dir"
            return 0
        fi
    done
    return 1
}

install_binary() {
    local src="$1"
    local dir=""

    dir=$(resolve_install_dir) || {
        print_error "No writable install directory found (tried: /usr/local/bin ${HOME}/.local/bin ${HOME}/bin)"
        return 1
    }

    chmod +x "$src"
    mv "$src" "${dir}/${BINARY_NAME}"
    echo "${dir}/${BINARY_NAME}"
}

ensure_path() {
    local dir="$1"
    case ":${PATH}:" in
        *":${dir}:"*) ;;
        *)
            export PATH="${dir}:${PATH}"
            print_warning "Added ${dir} to PATH for this session."
            print_warning "Add 'export PATH=\"${dir}:\$PATH\"' to your shell profile to make it permanent."
            ;;
    esac
}

installed_version() {
    local path output
    path=$(existing_install_path) || return 1
    output=$("$path" version 2>/dev/null) || return 1
    printf '%s\n' "$output" | sed -n -e 's/^Version:[[:space:]]*//p' -e 's/^版本:[[:space:]]*//p' | head -n 1
}

has_tty() {
    { : </dev/tty; } 2>/dev/null || return 1
    { : >/dev/tty; } 2>/dev/null || return 1
    return 0
}

# Ask a yes/no question on the terminal. Defaults to no, including when there
# is no terminal to ask on.
confirm() {
    local answer=""
    has_tty || return 1
    printf '%b' "${YELLOW}?${NC} $1 [y/N] " >/dev/tty
    read -r answer </dev/tty || return 1
    case "$answer" in
        y|Y|yes|Yes|YES) return 0 ;;
        *) return 1 ;;
    esac
}

# Return success when current is an older version than target. When version
# sorting is unavailable, any difference is treated as needing an update.
version_is_older() {
    local current="$1"
    local target="$2"

    [ -n "$current" ] || return 0
    [ "$current" = "$target" ] && return 1

    if printf '%s\n%s\n' "$current" "$target" | sort -V -C 2>/dev/null; then
        return 0
    fi
    if printf '%s\n%s\n' "$current" "$target" | sort -V >/dev/null 2>&1; then
        return 1
    fi
    return 0
}

run_binary() {
    local binary="$1"

    if ! "$binary" whoami >/dev/null 2>&1; then
        print_info "Not logged in; starting login to ${SERVER_URL}..."
        if [ -t 0 ]; then
            exec "$binary" login "$SERVER_URL"
        elif has_tty; then
            exec "$binary" login "$SERVER_URL" </dev/tty
        else
            print_warning "Not logged in. Run 'aist login ${SERVER_URL}' to continue."
            exit 0
        fi
    fi

    if [ -t 0 ]; then
        exec "$binary"
    elif has_tty; then
        exec "$binary" </dev/tty
    else
        exec "$binary"
    fi
}

# Download, verify and install one release. Prints the installed path.
install_release() {
    local version="$1"
    local os="$2"
    local arch="$3"
    local downloaded binary installed

    downloaded=$(download_archive "$version" "$os" "$arch") || return 1
    if ! verify_checksum "$downloaded" "$version"; then
        print_error "Download integrity check failed; not installing."
        rm -rf "$(dirname "$downloaded")"
        return 1
    fi

    if ! binary=$(extract_binary "$downloaded" "$version" "$os" "$arch"); then
        rm -rf "$(dirname "$downloaded")"
        return 1
    fi

    if ! installed=$(install_binary "$binary"); then
        rm -rf "$(dirname "$downloaded")"
        return 1
    fi
    rm -rf "$(dirname "$downloaded")"
    ensure_path "$(dirname "$installed")"
    printf '%s\n' "$installed"
}

# Install target and run it; falls back to the installed binary on failure.
switch_to_version() {
    local target="$1"
    local os="$2"
    local arch="$3"
    local current_path="$4"
    local current_version="$5"
    local installed_path=""

    if installed_path=$(install_release "$target" "$os" "$arch"); then
        print_success "Installed ${target}."
        run_binary "$installed_path"
    fi
    print_warning "Installation failed; running installed ${current_version:-aist}."
    run_binary "$current_path"
}

main() {
    echo ""
    echo "╔════════════════════════════════════════╗"
    echo "║   AIST RUN SCRIPT                      ║"
    echo "╚════════════════════════════════════════╝"
    echo ""

    if [ -z "$AIST_RELEASE_BASE_URL" ]; then
        print_error "AIST_RELEASE_BASE_URL is required."
        print_info "Example: AIST_RELEASE_BASE_URL=https://example.com/aist sh run.sh"
        exit 1
    fi

    local current_path=""
    local current_version=""
    if current_path=$(existing_install_path); then
        current_version=$(installed_version 2>/dev/null) || current_version=""
    else
        current_path=""
    fi

    local target_version=""
    print_info "Checking the latest version..."
    if ! target_version=$(resolve_latest_version); then
        target_version=""
    fi

    if [ -z "$target_version" ]; then
        if [ -n "$current_path" ]; then
            print_warning "Could not resolve the latest version; running installed ${current_version:-aist}."
            run_binary "$current_path"
        fi
        print_error "Could not resolve the latest version from ${AIST_RELEASE_BASE_URL}."
        print_info "Check that the server is reachable."
        exit 1
    fi

    local os
    local arch
    os=$(detect_os) || exit 1
    arch=$(detect_arch) || exit 1
    validate_platform "$os" "$arch" || exit 1

    if [ -z "$current_path" ]; then
        print_info "aist is not installed; installing ${target_version}."
        local installed_path=""
        if ! installed_path=$(install_release "$target_version" "$os" "$arch"); then
            print_error "Installation failed."
            exit 1
        fi
        print_success "Installed to ${installed_path}."
        run_binary "$installed_path"
    fi

    # The client must match the server: upgrades are silent because running the
    # script asks for the current client, while downgrades usually mean the
    # user switched servers and need confirmation.
    if version_is_older "$current_version" "$target_version"; then
        print_info "Installed version: ${current_version:-unknown}"
        print_info "Server version:    ${target_version}"
        print_info "Updating aist to ${target_version}..."
        switch_to_version "$target_version" "$os" "$arch" "$current_path" "$current_version"
    fi

    if [ "$current_version" != "$target_version" ]; then
        print_warning "Installed aist ${current_version} is newer than the server version ${target_version}."
        print_warning "The client should match the server version."
        if confirm "Install the server version ${target_version}?"; then
            switch_to_version "$target_version" "$os" "$arch" "$current_path" "$current_version"
        fi
        if has_tty; then
            print_info "Keeping installed ${current_version}."
        else
            print_info "Keeping installed ${current_version}; run this script in a terminal to install ${target_version}."
        fi
        run_binary "$current_path"
    fi

    print_success "aist ${current_version} is up to date."
    run_binary "$current_path"
}

main
