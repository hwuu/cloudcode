#!/bin/bash
# entrypoint.sh — devbox 容器入口脚本
# 管理两个进程：ttyd（Web Terminal）+ opencode（Web UI）

# ttyd 后台运行，崩溃自动重启
# 注：脚本以 opencode 用户执行（Dockerfile 中 USER opencode）
while true; do
    ttyd --writable --port 7681 --base-path /terminal /bin/bash
    sleep 1
done &

# opencode 作为主进程
exec opencode web --hostname 0.0.0.0 --port 4096
