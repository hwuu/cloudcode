#!/bin/bash
# CloudCode 安装脚本
# 用法: curl -fsSL https://github.com/hwuu/cloudcode/releases/latest/download/install.sh | bash
set -e

if ! command -v sudo &> /dev/null; then
  echo "错误: 需要 sudo 权限安装到 /usr/local/bin"
  exit 1
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH="amd64" ;; aarch64|arm64) ARCH="arm64" ;; esac

RELEASE_URL="https://github.com/hwuu/cloudcode/releases/latest/download/cloudcode-${OS}-${ARCH}"
echo "Downloading cloudcode for ${OS}/${ARCH}..."
curl -fsSL "$RELEASE_URL" | sudo tee /usr/local/bin/cloudcode > /dev/null
sudo chmod +x /usr/local/bin/cloudcode
echo "✅ cloudcode installed to /usr/local/bin/cloudcode"
echo "Run 'cloudcode --help' to get started"
