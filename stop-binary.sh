#!/bin/sh
set -eu

cd "$(dirname "$0")"

stop_pid_file() {
  pid_file="$1"
  if [ -f "$pid_file" ]; then
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$pid_file"
  fi
}

stop_pid_file logs/blog.pid
stop_pid_file logs/admin.pid

pkill -f "$(pwd)/blog_app" 2>/dev/null || true
pkill -f "$(pwd)/admin_app" 2>/dev/null || true

echo "Go-TeBlog stopped"
