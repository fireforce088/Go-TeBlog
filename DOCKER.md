# Docker Deployment

This project can run as one Docker container with both services:

- frontend service: `0.0.0.0:8190`
- admin service: `0.0.0.0:8191`
- public entry: `http://localhost:8190/blog`
- admin entry: `http://localhost:8190/admin`

## Quick Start

```bash
docker compose up -d --build
```

On first startup, if no administrator exists, the admin service creates a default account and prints the generated password in the container logs:

```text
username: admin
password: random 8-character alphanumeric value
```

Set an explicit initial password before first startup by editing `docker-compose.yml`:

```yaml
environment:
  INIT_ADMIN_USER: admin
  INIT_ADMIN_PASSWORD: change-this-password
```

The initialization account is only created when `/data/blog.sqlite` does not exist.

## Optional MinIO Image Storage

MinIO upload sync is disabled unless all required environment variables are set. When any required value is missing, uploads stay on local storage under `/data/uploads`.

```yaml
environment:
  MINIO_ENDPOINT: http://minio.example:9000
  MINIO_ACCESS_KEY: your-access-key
  MINIO_SECRET_KEY: your-secret-key
  MINIO_BUCKET: blog-images
  MINIO_PUBLIC_URL: https://img.example.com/blog-images
```

## Persistent Data

The compose file mounts local `./data` to container `/data`.

```text
data/
├── blog.sqlite
├── uploads/
└── backups/
```

To migrate an existing Typecho-compatible site:

1. Stop the container.
2. Put your SQLite database at `data/blog.sqlite`.
3. Put uploads under `data/uploads`.
4. Start the container again.

## Commands

Build and start:

```bash
docker compose up -d --build
```

View logs:

```bash
docker compose logs -f
```

Stop:

```bash
docker compose down
```

## Deployment Readiness Checklist

Before publishing a release or deploying from a clean checkout:

1. Confirm the release version:

```bash
cat VERSION
```

The current release version is `0.0.6`.

2. Build both services separately:

```bash
go build -o blog_app main.go
go build -o admin_app admin.go admin_helpers.go admin_storage.go
```

3. Run the available helper tests:

```bash
go test admin.go admin_helpers.go admin_storage.go admin_helpers_test.go
```

`go test ./...` is not a valid readiness check for the current repository layout because `main.go` and `admin.go` are separate entrypoints in the same `package main`; testing the whole package compiles both at once and reports duplicate definitions.

4. Ensure required source files are tracked:

```bash
git status --short
```

`admin_storage.go` is required by Docker, `build.sh`, and the admin binary build. A clean deployment from Git will fail if this file is left untracked.

5. For Docker deployment, verify the runtime image includes `curl`.

MinIO upload sync uses `curl` at runtime. The Dockerfile installs it with:

```dockerfile
RUN apk add --no-cache bash ca-certificates curl tar tzdata
```

6. For systemd deployment via `build.sh`, export MinIO variables before running the script if MinIO sync should be enabled:

```bash
export MINIO_ENDPOINT="http://minio.example:9000"
export MINIO_ACCESS_KEY="your-access-key"
export MINIO_SECRET_KEY="your-secret-key"
export MINIO_BUCKET="blog-images"
export MINIO_PUBLIC_URL="https://img.example.com/blog-images"
sudo bash build.sh
```

If these variables are omitted, the service still deploys and image uploads remain local.

## Notes

- The container exposes only port `8190` by default. The frontend proxies `/admin` to the internal admin service.
- The admin service still listens on `8191` inside the container.
- The in-app system restart button uses `systemctl`, so it is intended for the original systemd deployment and is not useful inside Docker.
