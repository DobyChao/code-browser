#!/bin/bash

set -e
LOG_DIR="./logs"
mkdir -p "$LOG_DIR"

# 先尝试关闭旧进程（避免重复启动）
./stop.sh >/dev/null 2>&1 || true

echo "🚀 启动 repo-server..."
nohup ./repo-server --admin-token "admin" > "$LOG_DIR/repo-server.log" 2>&1 &
REPO_PID=$!
echo $REPO_PID > "$LOG_DIR/repo-server.pid"
echo "   PID: $REPO_PID"

echo "🔍 启动 zoekt-webserver..."
nohup zoekt-webserver -index ./.data/zoekt-index/ -rpc > "$LOG_DIR/zoekt.log" 2>&1 &
ZOECT_PID=$!
echo $ZOECT_PID > "$LOG_DIR/zoekt.pid"
echo "   PID: $ZOECT_PID"

echo "✅ 启动完成！日志: $LOG_DIR"