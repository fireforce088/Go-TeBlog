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

The default initial admin account is:

```text
username: admin
password: admin
```

Change the initial password before first startup by editing `docker-compose.yml`:

```yaml
environment:
  INIT_ADMIN_USER: admin
  INIT_ADMIN_PASSWORD: change-this-password
```

The initialization account is only created when `/data/blog.sqlite` does not exist.

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

## Notes

- The container exposes only port `8190` by default. The frontend proxies `/admin` to the internal admin service.
- The admin service still listens on `8191` inside the container.
- The in-app system restart button uses `systemctl`, so it is intended for the original systemd deployment and is not useful inside Docker.
