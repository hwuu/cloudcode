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

# 设置 shell 补全
SHELL_NAME=$(basename "$SHELL")
COMPLETION_LINE='source <(cloudcode completion '"$SHELL_NAME"')'

case "$SHELL_NAME" in
  bash)
    RC_FILE="$HOME/.bashrc"
    ;;
  zsh)
    RC_FILE="$HOME/.zshrc"
    ;;
  *)
    echo "Run 'cloudcode --help' to get started"
    exit 0
    ;;
esac

if [ -f "$RC_FILE" ] && ! grep -q 'cloudcode completion' "$RC_FILE"; then
  echo "" >> "$RC_FILE"
  echo "# CloudCode shell completion" >> "$RC_FILE"
  echo "$COMPLETION_LINE" >> "$RC_FILE"
  echo "✅ Shell 补全已添加到 $RC_FILE（重新打开终端或执行 source $RC_FILE 生效）"
else
  echo "ℹ️  Shell 补全已存在于 $RC_FILE"
fi

echo "Run 'cloudcode --help' to get started"
