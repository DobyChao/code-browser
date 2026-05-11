#!/bin/bash

LOG_DIR="./logs"

stop_by_pid() {
    local pid_file="$1"
    local name="$2"
    if [ -f "$pid_file" ]; then
        pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            echo "⏹️  停止 $name (PID: $pid)..."
            kill "$pid"
            sleep 1
            # 强制清理
            kill -0 "$pid" && (sleep 1; kill -9 "$pid" 2>/dev/null || true) || true
        fi
        rm -f "$pid_file"
    fi
}

# 方法 1：尝试用 PID 文件关
stop_by_pid "$LOG_DIR/repo-server.pid" "repo-server"
stop_by_pid "$LOG_DIR/zoekt.pid" "old zoekt-webserver"

# 方法 2：兜底 —— 用 pkill 精准匹配命令（防止 PID 失效）
echo "🧹 清理残留进程..."
pkill -f "zoekt-webserver.*\.data/zoekt-index" 2>/dev/null && echo "   killed old zoekt-webserver" || true
pkill -f "\./repo-server$" 2>/dev/null && echo "   killed repo-server" || true

echo "✅ 所有服务已停止"
