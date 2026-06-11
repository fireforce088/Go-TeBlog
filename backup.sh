#!/usr/bin/env bash
set -euo pipefail

DATA_DIR="${DATA_DIR:-/data}"
DB_PATH="${DB_PATH:-$DATA_DIR/blog.sqlite}"
BACKUP_DIR="${BACKUP_DIR:-$DATA_DIR/backups}"

if [ ! -f "$DB_PATH" ]; then
  echo "Database not found: $DB_PATH" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"

sqlite3 "$DB_PATH" "PRAGMA wal_checkpoint(TRUNCATE);"

timestamp="$(date +%Y%m%d-%H%M%S)"
backup_path="$BACKUP_DIR/blog-$timestamp.sqlite"
cp "$DB_PATH" "$backup_path"

echo "$backup_path"
