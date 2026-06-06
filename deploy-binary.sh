#!/bin/sh
set -eu

cd "$(dirname "$0")"
mkdir -p logs backups usr/uploads

if [ ! -f usr/uploads/go.mod ]; then
  {
    echo "module attachments"
    echo "go 1.20"
  } > usr/uploads/go.mod
fi

./stop-binary.sh >/dev/null 2>&1 || true

if [ ! -f blog.sqlite ]; then
  timeout 8s ./blog_app > logs/init-blog.out.log 2> logs/init-blog.err.log || true
  ./admin_app --init-user="${INIT_ADMIN_USER:-admin}" --init-pass="${INIT_ADMIN_PASSWORD:-admin}"
fi

nohup ./admin_app > logs/admin.out.log 2> logs/admin.err.log &
echo "$!" > logs/admin.pid

nohup ./blog_app > logs/blog.out.log 2> logs/blog.err.log &
echo "$!" > logs/blog.pid

echo "Go-TeBlog started"
echo "Frontend: http://$(hostname -I 2>/dev/null | awk '{print $1}'):8190/blog"
echo "Admin:    http://$(hostname -I 2>/dev/null | awk '{print $1}'):8190/admin"
