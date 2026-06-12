# Changelog

## 0.0.11 - 2026-06-12

### Fixed

- **文章中使用 Obsidian 维基链接 `![[file.png]]` 导致图片不渲染**: 4 篇文章（CID 86/90/91/92）共 10 处使用 Obsidian 内链 `![[hash_MD5.png]]` 语法引用本地图片，Goldmark Markdown 渲染器不支持此语法，原样输出为纯文本。已统一替换为标准 Markdown 图片语法 `![](/blog/usr/uploads/nutstore/hash_MD5.png)`，并将图片从 Obsidian vault 附件目录复制到博客本地静态目录。受影响文章：薄膜技术（86）、DMA（90）、GCMS（91）、XPS（92）。

### Added

- **LaTeX 数学公式支持**: 引入 KaTeX CDN（v0.16.21）CSS + JS + auto-render 脚本，支持行内 `$...$` 和块级 `$$...$$` 公式渲染。
- **GFM 表格支持**: 启用 Goldmark `extension.Table`，支持标准 Markdown 管道表格渲染。
- **`protectLatexUnderscores()` 预处理**: 在 Goldmark 渲染前将 `$...$` / `$$...$$` 内部的 `_` 转义为 `\_`，避免被 Goldmark 误解析为 `<em>` 强调标记。

### Changed

- **KaTeX CDN 注入 `post.html` 模板**: 前台文章页面自动加载 KaTeX 相关资源，无需手动配置。

## 0.0.10 - 2026-06-12

### Changed

- **Docker 构建**: `Dockerfile` 中增加 `go mod tidy` 步骤，确保 go.sum 同步。

## 0.0.9 - 2026-06-12

### Fixed

- **图片外部链接被 fixAttachmentLinks 破坏**: `fixAttachmentLinks` 正则无条件 strip 掉所有外部域名的协议和域名前缀（如 `wikipedia.org` 等），导致博客中引用外部托管图片（如 Wikimedia Commons）的文章全部 404。v0.0.8 只加了 `img.w-tx.top` 白名单，未覆盖其他合法外部域名。现已彻底移除图库白名单/MinIO 重写逻辑：仅对以 `/usr/` 开头的 Typecho 默认附件路径做相对路径转换，**所有外链（http/https）一律保持原样**，包括 img.w-tx.top、公有图床等。

- **文章中 HTML 包裹的 Markdown 图片被 Goldmark 静默吞掉**: 16 篇旅行文章正文使用 `<p align="center">![alt](url)</p>` 混合格式渲染。Goldmark（CommonMark 规范）在 HTML 块内不解析 Markdown 语法，导致 `![alt](url)` 被当作纯文本输出 HTML 注释。新增 `unwrapHTMLImages()` 函数在渲染前将此类模式解包为纯 Markdown 格式。

- **后台编辑器加载含 HTML 文章时无法正确解析 Markdown**: 从外部迁移或旧编辑器保存的文章正文可能混入 `<p>`、`<img>`、`<strong>`、`<h2>` 等 HTML 标签，导致在后台编辑器中以原始 HTML 源码显示。新增 `normalizeEditorMarkdown()` 函数，在后台编辑页面加载时自动将常见 HTML 标签转换为 Markdown 格式（包括标题、段落、加粗、斜体、链接、图片、列表、引用、代码块、HTML 实体解码等），不影响前台渲染路径。

### Changed

- 移除 `html.WithUnsafe()` 渲染选项：Goldmark 不再允许 Markdown 中嵌入原始 HTML，后续应使用纯 Markdown 或 Template 方式插入富文本。
- renderMarkdown 中增加 `<!--more-->` 标签清除，防止摘录截断标记在全文页原样显示。
- `main.go`：`fixAttachmentLinks` 移除 `img.w-tx.top` 白名单判断，不再因 `MINIO_PUBLIC_URL` / `PUBLIC_URL` 重写外链。所有 `http://` 和 `https://` 图片链接保持原样。
- `admin.go`：编辑文章路由在加载文章到编辑器时调用 `normalizeEditorMarkdown()`，将 HTML 混入内容在编辑器中自动转 Markdown。

### Added

- **`editor_normalize.go`**：新增 HTML→Markdown 独立转换模块（~250 行，纯标准库实现）。支持：标题（h1~h6）、段落、换行、加粗/斜体、链接、图片、有序/无序列表、引用、代码块（含语言类）、HTML 实体解码。无法可靠转换的标签提取文本内容后丢弃。
- **`editor_normalize_test.go`**：16 个单元测试覆盖所有转换规则和边界情况，以及外链保持原样验收用例。

## 0.0.8 - 2026-06-11

### Fixed

- **Broken image URLs after Markdown conversion**: `fixAttachmentLinks()` in `main.go` was converting `img.w-tx.top` absolute URLs to broken relative paths (`/blog-images/...`). The blog has no route for `/blog-images/`, and local files use MD5-hashed names, not Chinese filenames. Added domain skip: `img.w-tx.top` URLs now remain as absolute `https://` URLs, served directly from the still-running MinIO at HK-CN2. (31 posts affected.)

## 0.0.7 - 2026-06-11

### Fixed

- **Admin & blog apps**: Added `--db` flag to specify database file path, replacing previously hardcoded `./blog.sqlite`. The flag is available in both `admin_app` and `blog_app`, allowing standalone CLI operations (e.g., password reset) to target any database path without requiring the binary to run from a specific directory.
- **blog_app**: Added `--init-user`, `--init-pass`, `--reset-password`, `--reset-user`, `--reset-pass` flags (previously only available in `admin_app`), so blog_app can handle admin password management directly when used standalone or via `docker exec`.
- **Docker entrypoint**: Added `RESET_ADMIN_USER` / `RESET_ADMIN_PASSWORD` environment variables for runtime password reset without deleting the database. Both `admin_app` and `blog_app` now receive the `--db` path in the entrypoint script, ensuring correct database targeting regardless of working directory.
- **SQLite WAL backup**: Added `backup.sh` script that performs `PRAGMA wal_checkpoint(TRUNCATE)` before copying the database file, preventing data loss when backing up a WAL-mode SQLite database via regular `cp`.
- **Dockerfile**: Added `sqlite` package and `backup.sh` to the container image.

### Changed

- Database connection is now opened after `flag.Parse()` in both `admin.go` and `main.go`, so the `--db` flag value is available at connection time.
- Bumped version from `0.0.6` to `0.0.7`.

## 0.0.6 - 2026-06-10

### Added

- Added first-run admin account provisioning: when no administrator exists, the admin service creates the default `admin` user with a random 8-character alphanumeric password and prints the credentials in the backend startup logs.
- Added `--reset-password` command for `admin_app`, with optional `--reset-user` and `--reset-pass` flags for resetting an admin password from the command line.
- Added test coverage for generated admin password format.

### Changed

- Changed Docker first-run defaults so `admin/admin` is no longer used implicitly; unset `INIT_ADMIN_PASSWORD` now lets `admin_app` generate a random initial password.
- Updated the release version to `0.0.6`.

## 0.0.5 - 2026-06-10

### Security

- Added CSRF protection for admin login and authenticated admin POST requests.
- Disabled unsafe raw HTML rendering in Markdown output to reduce stored XSS exposure.
- Migrated admin password hashing to bcrypt while keeping legacy Phpass hashes login-compatible and rehashing them after successful login.
- Set admin session and CSRF cookies as `Secure` when requests arrive through an HTTPS reverse proxy.
- Removed hardcoded MinIO endpoint/access-key/public URL defaults; MinIO now requires environment configuration and falls back to local storage when incomplete.

### Removed

- Removed the unreleased admin CA protection shell and the dependency on an outer Cloudflare Access layer.
- Restored the built-in admin username/password login, session cookie validation, login rate limiting, and logout session cleanup as the primary admin protection.

### Changed

- Added Docker Compose pass-through entries for MinIO environment variables without hardcoding sensitive defaults.
- Updated the release version to `0.0.5`.

## 0.0.4 - 2026-06-09

### Added

- Added MinIO image storage integration: uploaded images are synced to MinIO bucket `blog-images`, served via CDN at `https://img.w-tx.top/blog-images/...`.
- Added `-addr` CLI flag to admin_app for custom listen address (supports staging alongside production on same host).
- Added `MINIO_SECRET_KEY` / `MINIO_ENDPOINT` / `MINIO_ACCESS_KEY` environment variable support (credentials no longer hardcoded in source).
- Added automatic Content-Type detection for MinIO uploads (jpeg/png/gif/webp/svg/bmp/ico).
- Added `DeleteFromMinIO()` helper for future attachment management.
- Added deployment log convention: `/vol1/1000/dev/<project>/deploy-logs/`.

### Changed

- Refactored MinIO configuration from constants to runtime variables with env var override.
- Updated the release version to `0.0.4`.

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
