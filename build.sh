#!/usr/bin/env bash
# 交叉编译脚本，产物放在 dist/<os>-<arch>/
# 用法: ./build.sh [linux|darwin] [amd64|arm64]
set -euo pipefail
cd "$(dirname "$0")"

OS=${1:-linux}
ARCH=${2:-amd64}
OUT="dist/${OS}-${ARCH}"
LDFLAGS="-s -w"
export CGO_ENABLED=0

mkdir -p "$OUT"
echo "构建 $OS/$ARCH -> $OUT"
GOOS=$OS GOARCH=$ARCH go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/gpu-panel-server" ./cmd/server
GOOS=$OS GOARCH=$ARCH go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/gpu-panel-agent" ./cmd/agent

echo "完成:"
ls -lh "$OUT"
