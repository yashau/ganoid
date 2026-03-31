#!/usr/bin/env bash
# Ganoid Build Script
#
# Usage:
#   ./build.sh 0.1.0
#   ./build.sh 0.1.0 all
#   ./build.sh 0.1.0 linux

set -euo pipefail

VERSION="${1:-}"
TARGET="${2:-windows}"

if [ -z "$VERSION" ]; then
    echo "Error: VERSION is required"
    echo "Usage: ./build.sh VERSION [TARGET]"
    echo "  TARGET: windows | linux | darwin | all (default: windows)"
    exit 1
fi

BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION"
MAJOR="${MAJOR:-0}"; MINOR="${MINOR:-1}"; PATCH="${PATCH:-0}"
VERSION_STRING="${MAJOR}.${MINOR}.${PATCH}.0"

LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo ""
echo -e "\033[36mBuilding Ganoid\033[0m"
echo "  Version:    ${VERSION}"
echo "  Build time: ${BUILD_TIME}"
echo "  Git commit: ${GIT_COMMIT}"
echo "  Target:     ${TARGET}"
echo ""

cd "$SCRIPT_DIR"

# ── Build SvelteKit UI ────────────────────────────────────────────────────────
echo -e "\033[33mBuilding UI...\033[0m"
(cd ui && pnpm install --frozen-lockfile && pnpm run build)
echo -e "  \033[32mOK  UI built\033[0m"

# ── Generate versioninfo.json + resource.syso for one binary ─────────────────
gen_resource() {
    local CMD_DIR="$1" BIN_NAME="$2" DESCRIPTION="$3" ORIG_NAME="$4"

    cat > "${CMD_DIR}/versioninfo.json" <<EOF
{
  "FixedFileInfo": {
    "FileVersion":    { "Major": ${MAJOR}, "Minor": ${MINOR}, "Patch": ${PATCH}, "Build": 0 },
    "ProductVersion": { "Major": ${MAJOR}, "Minor": ${MINOR}, "Patch": ${PATCH}, "Build": 0 },
    "FileFlagsMask": "3f", "FileFlags": "00", "FileOS": "040004",
    "FileType": "01", "FileSubType": "00"
  },
  "StringFileInfo": {
    "Comments": "", "CompanyName": "Ibrahim Yashau",
    "FileDescription": "${DESCRIPTION}",
    "FileVersion": "${VERSION_STRING}",
    "InternalName": "${BIN_NAME}",
    "LegalCopyright": "Copyright (c) 2026 Ibrahim Yashau. All rights reserved.", "LegalTrademarks": "",
    "OriginalFilename": "${ORIG_NAME}",
    "PrivateBuild": "", "ProductName": "Ganoid",
    "ProductVersion": "${VERSION_STRING}", "SpecialBuild": ""
  },
  "VarFileInfo": { "Translation": { "LangID": "0409", "CharsetID": "04B0" } },
  "IconPath": "../../internal/tray/icon.ico",
  "ManifestPath": ""
}
EOF

    if command -v goversioninfo &>/dev/null; then
        (cd "$CMD_DIR" && goversioninfo -64 -o resource.syso versioninfo.json) \
            && echo -e "  \033[32mOK  resource.syso (${BIN_NAME})\033[0m" \
            || echo -e "  \033[33mWARN goversioninfo failed for ${BIN_NAME}\033[0m"
    fi
}

# ── Build one OS/arch pair ────────────────────────────────────────────────────
build_pair() {
    local OS="$1" ARCH="$2" OUTD="$3" OUTG="$4"

    echo -e "\033[33mBuilding ganoidd (${OS}/${ARCH})...\033[0m"
    CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" go build -ldflags "${LDFLAGS}" -o "$OUTD" ./cmd/ganoidd
    echo -e "  \033[32mOK  ${OUTD}\033[0m"

    echo -e "\033[33mBuilding ganoid (${OS}/${ARCH})...\033[0m"
    CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" go build -ldflags "${LDFLAGS}" -o "$OUTG" ./cmd/ganoid
    echo -e "  \033[32mOK  ${OUTG}\033[0m"
}

# ── Windows resources (only when building for windows) ───────────────────────
if [ "$TARGET" = "windows" ] || [ "$TARGET" = "all" ]; then
    echo -e "\033[33mGenerating Windows resources...\033[0m"
    gen_resource "${SCRIPT_DIR}/cmd/ganoidd" "ganoidd" \
        "Ganoid Daemon — Tailscale profile coordination server" "ganoidd.exe"
    gen_resource "${SCRIPT_DIR}/cmd/ganoid" "ganoid" \
        "Ganoid — Tailscale profile manager tray application" "ganoid.exe"
fi

# ── Compile ───────────────────────────────────────────────────────────────────
case "$TARGET" in
    windows) build_pair windows amd64 ganoidd.exe    ganoid.exe    ;;
    linux)   build_pair linux   amd64 ganoidd-linux  ganoid-linux  ;;
    darwin)  build_pair darwin  arm64 ganoidd-darwin ganoid-darwin ;;
    all)
        build_pair windows amd64 ganoidd.exe    ganoid.exe
        build_pair linux   amd64 ganoidd-linux  ganoid-linux
        build_pair darwin  arm64 ganoidd-darwin ganoid-darwin
        ;;
    *)
        echo "Unknown target: $TARGET"
        exit 1
        ;;
esac

# ── Checksums ─────────────────────────────────────────────────────────────────
echo -e "\033[33mGenerating checksums...\033[0m"
BINS=()
case "$TARGET" in
    windows) BINS=(ganoidd.exe    ganoid.exe)    ;;
    linux)   BINS=(ganoidd-linux  ganoid-linux)  ;;
    darwin)  BINS=(ganoidd-darwin ganoid-darwin) ;;
    all)     BINS=(ganoidd.exe ganoid.exe ganoidd-linux ganoid-linux ganoidd-darwin ganoid-darwin) ;;
esac

true > checksums.txt
for b in "${BINS[@]}"; do
    if [ -f "$b" ]; then
        sha256sum "$b" | awk '{print $1"  "$2}' >> checksums.txt
        echo -e "  \033[32m${b}\033[0m"
    fi
done
echo -e "  Checksums written to checksums.txt"

echo ""
echo -e "\033[32mBuild successful! Ganoid v${VERSION}\033[0m"
