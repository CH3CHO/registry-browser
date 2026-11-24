#!/usr/bin/env bash
# 在 WSL Ubuntu 里跑带 race detector 的测试。
#
# Windows 上的 race detector 需要 cgo 与 C 编译器；本项目不安装 MinGW，
# 改为借用 WSL（自带 gcc）。Go 工具链与所有缓存都放在 D 盘，避免
# WSL 虚拟磁盘（ext4.vhdx）把系统盘撑满。
#
# 用法（在 WSL 里）：
#   bash /mnt/d/path/to/registry-browser/scripts/run-race-wsl.sh
#
# 前置：
#   - WSL Ubuntu 已安装 gcc（sudo apt-get install -y build-essential）
#   - Go 已安装到 D 盘（默认 /mnt/d/Tools/go），没有则脚本会提示下载地址
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.26.2}"
TOOLS_DIR="${TOOLS_DIR:-/mnt/d/Tools}"
CACHE_DIR="${CACHE_DIR:-/mnt/d/Temp/go}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export PATH="${TOOLS_DIR}/go/bin:${PATH}"
export GOMODCACHE="${CACHE_DIR}/pkg/mod"
export GOCACHE="${CACHE_DIR}/build"
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}"
export GOFLAGS="-mod=mod"
export CGO_ENABLED=1

if ! command -v gcc >/dev/null 2>&1; then
  echo "错误：WSL 里没有 gcc，请先执行 sudo apt-get install -y build-essential" >&2
  exit 1
fi

if [ ! -x "${TOOLS_DIR}/go/bin/go" ]; then
  echo "错误：${TOOLS_DIR}/go 不存在。请下载 go${GO_VERSION}.linux-amd64.tar.gz 解压到 ${TOOLS_DIR}。" >&2
  echo "下载地址：https://golang.google.cn/dl/go${GO_VERSION}.linux-amd64.tar.gz" >&2
  exit 1
fi

mkdir -p "${GOMODCACHE}" "${GOCACHE}"

go version
gcc --version | sed -n '1p'
echo "GOMODCACHE=${GOMODCACHE}"
echo "GOCACHE=${GOCACHE}"

cd "${REPO_DIR}"

echo "== go vet =="
go vet ./internal/... . || true

echo "== go test -race =="
go test -race -count=1 ./internal/...
