# Changelog

## 0.0.1 - 2026-06-06

### Added

- Added Docker single-container deployment files: `Dockerfile`, `docker-compose.yml`, `.dockerignore`, and `docker-entrypoint.sh`.
- Added Docker deployment guide in `DOCKER.md`.
- Added binary deployment scripts: `deploy-binary.sh` and `stop-binary.sh`.
- Added Markdown file drag-and-drop import support for the admin post editor.
- Added `VERSION` file with release version `0.0.1`.

### Fixed

- Changed the frontend-to-admin proxy target to `127.0.0.1:8191` for local dual-process and Docker runtime compatibility.
- Fixed frontend template year rendering.
