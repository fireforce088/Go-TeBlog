# Changelog

## 0.0.3 - 2026-06-07

### Added

- Added category password protection with backend route-level access checks.
- Added admin category controls for enabling access passwords and setting bcrypt-hashed passwords.
- Added a frontend category password page for protected categories.
- Added automatic database migration columns: `protected` and `password_hash` in `go_category_settings`.

### Changed

- Excluded protected-category posts from public homepage, search, archive, sitemap, sidebar, and previous/next post listings.
- Updated the release version to `0.0.3`.

## 0.0.2 - 2026-06-07

### Added

- Added a wide three-column frontend layout for desktop pages.
- Added expandable category article lists in the frontend sidebar.
- Added current category and current article highlighting in navigation.

### Changed

- Increased the default frontend content width to better use large screens.
- Updated the release version to `0.0.2`.

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
