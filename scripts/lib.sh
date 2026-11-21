#!/usr/bin/env bash

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

resolve_version() {
    local raw="${VERSION:-}"

    if [ -z "$raw" ]; then
        raw=$(git tag --list "v*" --sort=v:refname | tail -n 1)
    fi
    if [ -z "$raw" ]; then
        raw="dev"
    fi
    printf '%s\n' "$raw"
}

build_commit() {
    printf '%s\n' "${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
}

build_date() {
    printf '%s\n' "${BUILD_DATE:-$(date -u +"%Y-%m-%d")}"
}

build_binary() {
    local package="$1"
    local binary_name="$2"
    local ldflags="$3"
    local os="$4"
    local arch="$5"
    local out_dir="$6"
    local output="${out_dir}/${binary_name}-${os}-${arch}"

    local goarm=""
    if [ "$arch" = "arm" ]; then
        goarm="6"
    fi

    echo -e "${BLUE}Building ${binary_name} for ${os}/${arch}${goarm:+ (GOARM=${goarm})}...${NC}"
    if ! CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOARM=$goarm go build -ldflags "${ldflags}" -trimpath -o "$output" "$package"; then
        echo -e "${YELLOW}⚠ Build failed for ${binary_name} ${os}/${arch}, skipping...${NC}"
        echo ""
        return 1
    fi

    if [ ! -f "$output" ]; then
        echo "✗ Failed to build ${binary_name} for ${os}/${arch}"
        echo ""
        return 1
    fi

    echo -e "${GREEN}✓${NC} Built: $output ($(du -h "$output" | cut -f1))"
    echo ""
}

archive_binary() {
    local dir="$1"
    local binary_name="$2"
    local os="$3"
    local arch="$4"
    local base="${binary_name}-${os}-${arch}"

    if [ -f "${dir}/${base}" ]; then
        (cd "$dir" && tar czf "${base}.tar.gz" "${base}")
    fi
}

write_checksums() {
    local dir="$1"
    local pattern="$2"

    (cd "$dir" && {
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum ${pattern}
        else
            shasum -a 256 ${pattern}
        fi
    } > checksums.txt)
}

write_latest() {
    local dir="$1"
    local version="$2"

    printf '%s\n' "$version" > "${dir}/latest.txt"
    printf '{"version":"%s"}\n' "$version" > "${dir}/latest.json"
}
