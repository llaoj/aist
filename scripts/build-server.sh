#!/usr/bin/env bash

# Build the AIST server for container platforms and assemble a release bundle
# under dist/server/.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib.sh
. "${SCRIPT_DIR}/lib.sh"

export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

VERSION=$(resolve_version)
COMMIT=$(build_commit)
BUILD_DATE=$(build_date)

SERVER_NAME="server-${VERSION}"
RELEASE_DIR="${ROOT_DIR}/dist/server/${VERSION}"
SERVER_LDFLAGS="-X main.BuildVersion=${VERSION} -X main.BuildCommit=${COMMIT} -X main.BuildDate=${BUILD_DATE} -s -w"

PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
)

echo -e "${BLUE}Building ${SERVER_NAME} version ${VERSION}${NC}"
echo ""

echo -e "${BLUE}Preparing release directory...${NC}"
rm -rf "$RELEASE_DIR"
mkdir -p "$RELEASE_DIR"
echo ""

for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r os arch <<< "$platform"
    build_binary "./cmd/server/" "$SERVER_NAME" "$SERVER_LDFLAGS" "$os" "$arch" "$RELEASE_DIR" || true
done

echo -e "${BLUE}Creating release archives...${NC}"
for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r os arch <<< "$platform"
    archive_binary "$RELEASE_DIR" "$SERVER_NAME" "$os" "$arch"
done

echo -e "${BLUE}Generating checksums...${NC}"
write_checksums "$RELEASE_DIR" "server-*"

echo -e "${BLUE}Writing latest version pointer...${NC}"
write_latest "${ROOT_DIR}/dist/server" "$VERSION"

echo -e "${GREEN}Server build completed!${NC}"
echo ""
echo "Release directory: $RELEASE_DIR"
echo "Files:             $(ls -1 "$RELEASE_DIR" | wc -l | tr -d ' ')"
