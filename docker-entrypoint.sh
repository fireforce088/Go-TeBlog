#!/usr/bin/env bash
set -euo pipefail

DATA_DIR="${DATA_DIR:-/data}"
INIT_ADMIN_USER="${INIT_ADMIN_USER:-admin}"
INIT_ADMIN_PASSWORD="${INIT_ADMIN_PASSWORD:-}"
RESET_ADMIN_USER="${RESET_ADMIN_USER:-$INIT_ADMIN_USER}"
RESET_ADMIN_PASSWORD="${RESET_ADMIN_PASSWORD:-}"

mkdir -p "$DATA_DIR/uploads" "$DATA_DIR/backups"

if [ ! -f "$DATA_DIR/uploads/go.mod" ]; then
  {
    echo "module attachments"
    echo "go 1.20"
  } > "$DATA_DIR/uploads/go.mod"
fi

rm -rf /app/usr/uploads /app/backups
ln -s "$DATA_DIR/uploads" /app/usr/uploads
ln -s "$DATA_DIR/backups" /app/backups

if [ ! -e /app/blog.sqlite ]; then
  ln -s "$DATA_DIR/blog.sqlite" /app/blog.sqlite
fi

if [ ! -f "$DATA_DIR/blog.sqlite" ] && [ -n "$INIT_ADMIN_PASSWORD" ]; then
  /app/admin_app --db="$DATA_DIR/blog.sqlite" --init-user="$INIT_ADMIN_USER" --init-pass="$INIT_ADMIN_PASSWORD"
fi

if [ -n "$RESET_ADMIN_PASSWORD" ]; then
  /app/admin_app --db="$DATA_DIR/blog.sqlite" --reset-password --reset-user="$RESET_ADMIN_USER" --reset-pass="$RESET_ADMIN_PASSWORD"
fi

shutdown() {
  if [ -n "${BLOG_PID:-}" ]; then
    kill "$BLOG_PID" 2>/dev/null || true
  fi
  if [ -n "${ADMIN_PID:-}" ]; then
    kill "$ADMIN_PID" 2>/dev/null || true
  fi
  wait 2>/dev/null || true
}

trap shutdown INT TERM

/app/admin_app --db="$DATA_DIR/blog.sqlite" &
ADMIN_PID="$!"

/app/blog_app --db="$DATA_DIR/blog.sqlite" &
BLOG_PID="$!"

wait -n "$ADMIN_PID" "$BLOG_PID"
shutdown
