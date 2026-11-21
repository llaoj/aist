#!/bin/sh

# Uninstall aist for the current user.
#
# Removes the installed aist binary and the client state root (~/.aist).
#
#   curl -fsSL https://aistrun.cc/uninstall.sh | sh
#   curl -fsSL https://aistrun.cc/uninstall.sh | sh -s -- --yes
#
# Environment:
#   AIST_YES=1  skip the confirmation prompt

set -eu

BINARY_NAME="aist"
STATE_DIR_NAME=".aist"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ASSUME_YES="${AIST_YES:-}"
SEEN_TARGETS=""

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

usage() {
    cat <<'EOF'
Usage: uninstall.sh [--yes]

Remove the aist client binary and the client state root (~/.aist).

Options:
  -y, --yes   skip the confirmation prompt
  -h, --help  show this help
EOF
}

has_tty() {
    { : </dev/tty; } 2>/dev/null || return 1
    { : >/dev/tty; } 2>/dev/null || return 1
    return 0
}

add_target() {
    local target="$1"
    [ -n "$target" ] || return 0
    case "$SEEN_TARGETS" in
        *"|${target}|"*) return 0 ;;
    esac
    SEEN_TARGETS="${SEEN_TARGETS}|${target}|"
    printf '%s\n' "$target"
}

# Follow symlinks and return the final path.
resolve_link_target() {
    local path="$1"
    local link

    while [ -L "$path" ]; do
        link=$(readlink "$path")
        case "$link" in
            /*) path="$link" ;;
            *) path="$(dirname "$path")/$link" ;;
        esac
    done
    printf '%s\n' "$path"
}

# Collect aist binaries from PATH and the directories used by run.sh.
find_install_targets() {
    local path
    local dir
    local resolved

    path=$(command -v "$BINARY_NAME" 2>/dev/null) || path=""
    if [ -n "$path" ] && { [ -e "$path" ] || [ -L "$path" ]; }; then
        add_target "$path"
        if [ -L "$path" ]; then
            resolved=$(resolve_link_target "$path")
            if [ -n "$resolved" ] && [ "$(basename "$resolved")" = "$BINARY_NAME" ]; then
                add_target "$resolved"
            elif [ -n "$resolved" ]; then
                print_warning "Leaving symlink target untouched: ${resolved}"
            fi
        fi
    fi

    for dir in "/usr/local/bin" "${HOME:-}/.local/bin" "${HOME:-}/bin"; do
        path="${dir}/${BINARY_NAME}"
        if [ -e "$path" ] || [ -L "$path" ]; then
            add_target "$path"
        fi
    done
}

confirm_removal() {
    local answer

    if [ "$ASSUME_YES" = "1" ]; then
        return 0
    fi

    printf '%b' "${YELLOW}?${NC} Remove the paths listed above? [y/N] " >/dev/tty
    if ! read -r answer </dev/tty; then
        return 1
    fi
    case "$answer" in
        y|Y|yes|YES) return 0 ;;
        *) return 1 ;;
    esac
}

main() {
    local arg
    local home="${HOME:-}"
    local state_dir
    local targets
    local target
    local failed=0

    while [ "$#" -gt 0 ]; do
        arg="$1"
        case "$arg" in
            -y|--yes) ASSUME_YES=1 ;;
            -h|--help) usage; exit 0 ;;
            *)
                print_error "Unknown option: ${arg}"
                usage >&2
                exit 1
                ;;
        esac
        shift
    done

    echo ""
    echo "╔════════════════════════════════════════╗"
    echo "║   AIST UNINSTALL SCRIPT                 ║"
    echo "╚════════════════════════════════════════╝"
    echo ""

    if [ -z "$home" ] || [ "$home" = "/" ]; then
        print_error "Refusing to continue: HOME is not set or is /."
        exit 1
    fi

    state_dir="${home}/${STATE_DIR_NAME}"

    targets=$(find_install_targets)

    if [ -z "$targets" ] && [ ! -e "$state_dir" ] && [ ! -L "$state_dir" ]; then
        print_success "aist is not installed; nothing to remove."
        exit 0
    fi

    print_info "The following paths will be removed:"
    if [ -n "$targets" ]; then
        while IFS= read -r target; do
            [ -n "$target" ] && printf '    %s\n' "$target" >&2
        done <<EOF
$targets
EOF
    fi
    if [ -e "$state_dir" ] || [ -L "$state_dir" ]; then
        printf '    %s\n' "$state_dir" >&2
    fi

    if [ "$ASSUME_YES" != "1" ] && ! has_tty; then
        print_error "No interactive terminal detected; refusing to remove anything."
        print_info "Re-run with --yes to skip confirmation:"
        print_info "  curl -fsSL https://aistrun.cc/uninstall.sh | sh -s -- --yes"
        exit 1
    fi

    if ! confirm_removal; then
        print_info "Uninstall cancelled."
        exit 0
    fi

    if [ -n "$targets" ]; then
        while IFS= read -r target; do
            [ -n "$target" ] || continue
            if rm -f "$target"; then
                print_success "Removed ${target}"
            else
                print_error "Failed to remove ${target}"
                failed=1
            fi
        done <<EOF
$targets
EOF
    fi

    if [ -e "$state_dir" ] || [ -L "$state_dir" ]; then
        if rm -rf "$state_dir"; then
            print_success "Removed ${state_dir}"
        else
            print_error "Failed to remove ${state_dir}"
            failed=1
        fi
    fi

    if [ "$failed" -ne 0 ]; then
        print_error "Uninstall completed with errors."
        exit 1
    fi

    print_success "aist has been uninstalled."
}

main "$@"
