# Changelog

## 0.1.8 - 2026-06-17

### Added

- **摘要模式**: 新增 `excerpt` 模板函数，首页文章自动截断 300 字符（支持 `<!--more-->` 标签），长文不再全文平铺。
- **琥珀色点缀色**: CSS 新增 `--accent-color: #d97706`，技术博客品牌色。

### Changed

- **字体与阅读体验**: 正文字号 14px → 15px，行高 1.5 → 1.7，背景色调暖（`#f8fafc`）。
- **代码块深色化**: Light 模式改用 Catppuccin Mocha 深色背景（`#1e1e2e`），Dark 模式改用 GitHub Dark（`#0d1117`），圆角+边框优化。
- **配色更新**: 主色从 `#3b82f6` 改为 `#2563eb`，暗色模式背景从 gray-800 改为 slate-900 系（更柔和）。
- **阴影优化**: 卡片阴影值微调，hover 动效保留。

### Fixed

- 首页长文章不再撑满多屏。

## 0.1.7 - 2026-06-14

### Added

- **统一图片处理包**: 将 `image_localizer.go`、`image_searcher.go` 整合为 `internal/image/` 包，包含统一配置（`config.go`）、本地化（`localizer.go`）、SSRF防护（`security.go`）、存储（`storage.go`）、Markdown/HTML 解析（`parser.go`）、在线搜索替换（`searcher.go`）、整合入口（`searchfix.go`）。对外只暴露 `image.ProcessContent()`, 先本地化再搜索替换。

### Changed

- **存储路径更改**: 图片保存路径从 `./usr/uploads/article-images/` 改为 `/data/blog-images/`，公开 URL 前缀从 `/usr/uploads/` 改为 `/blog-images/`。宿主机对应路径 `/vol1/1000/Docker/Go-Blog/blog-images/`。
- **`admin.go` POST /save**: 改用 `image.ProcessContent(ctx, text)` 统一入口，替代 `ImageLocalizer.Localize()` + `FixArticleImages()` 的分别调用。
- **Dockerfile/build.sh**: 移除逐文件构建列表，适配新包结构。
- **`scripts/travel_image_picker.py`**: 新增 `--download-dir` 参数，支持下载选中图片到本地 `blog-images/` 目录并替换输出中的远程 URL 为本地路径，默认目录 `/vol1/1000/Docker/Go-Blog/blog-images/`。
- **`proxyapps` 用户**: 创建专用 UID 986 用于透明代理，TPROXY 规则已验证通过。普通用户直连，proxyapps 用户走代理（`82.152.91.229`）。

### Removed

- 根目录 `image_localizer.go`、`image_searcher.go`（已迁入 `internal/image/`）。
- `LocalizeRemoteImages()`、`FixAndLocalizeImages()`（被 `image.ProcessContent()` 替代）。

## 0.1.5 - 2026-06-13

### Added

- **远程图片搜索替换**: 新增 `image_searcher.go`，后台保存文章时对下载失败的远程图片自动搜索 Wikimedia Commons 替换。新增 `cmd/img-fixer/` HK-CN2 Worker，在港区 VPS 上执行 Commons 搜索 → 下载 → MinIO 上传，通过 Tailscale HTTP API 供博客端调用。新增环境变量 `IMAGE_SEARCH_ENABLED`（默认 true）、`IMAGE_SEARCH_WORKER_URL`（Worker 端点）。
- **HTML `<img>` 标签支持**: `image_localizer.go` 新增 HTML `<img src="">` 图片标签处理（原仅支持 Markdown `![]()`），保存文章时自动下载 HTML 图片到本地存储。

### Architecture

- **HK-CN2 Image Fixer Worker**: 独立 Go 二进制（`cmd/img-fixer/main.go`），监听 `:8900`，提供 `POST /search` API。通过 systemd 管理，使用 `mc` CLI 上传到本地 MinIO。
- **Tailscale 桥接**: xm-50 博客容器通过 Tailscale（100.119.183.123:8900）调用 HK-CN2 Worker，突破 GFW 对 Wikimedia 的封锁。

## 0.1.4 - 2026-06-13

### Added

- **远程图片本地化**: 后台保存文章时自动下载远程 Markdown 图片到本地 `uploads/images/YYYY/MM/` 目录，替换链接为本地路径。新增 `image_localizer.go`。可通过环境变量 `IMAGE_LOCALIZE_ENABLED`（默认 true）、`IMAGE_LOCALIZE_MAX_SIZE_MB`（默认 10）、`IMAGE_LOCALIZE_TIMEOUT_SEC`（默认 15）、`IMAGE_LOCALIZE_DIR`（默认 uploads/images）配置。安全限制：禁止内网 IP、只允许 jpg/png/webp/gif、Content-Type + magic bytes 双重校验。下载失败保留原链接。

## 0.1.3 - 2026-06-12

### Removed

- **Cloudflare 五秒盾**: 移除项目内置的 Cloudflare 五秒盾/访问防护功能。包括：自动安全等级切换、IP 自动拉黑、流量阈值检测、五秒盾中间件、相关路由、数据库表、设置页面配置项、仪表盘日志面板。外部 Cloudflare Tunnel/Access 不受影响。
- **AI 评论检测**: 移除 AI 评论垃圾检测和 AI 攻击类型分析功能。包括：评论提交时的 AI 评分、AI 测试接口、AI 系统设置配置项、所有相关后台函数（checkSpamAI、callAIChatCompletionText、extractSpamScore 等）。普通评论审核功能保留。
- **文档清理**: README.md 和 PROJECT_GUIDE.md 中相关功能描述已删除。

## 0.1.2 - 2026-06-12

### Added

- **文章管理分类筛选**: 在文章管理列表页面增加分类下拉框，支持按分类筛选文章。三个筛选条件（标题关键词 + 分类 + 状态）可组合使用。查询使用参数化 EXISTS 子查询，防 SQL 注入。筛选后条件保持，分页参数完整保留。无匹配时显示友好提示。

### Changed

- **admin_helpers.go**: `AdminPostFilter` 加 `CategoryID` 字段、新增 `parseAdminCategoryID()` 函数、`buildPostFilterWhere` 加分类 EXISTS 子查询
- **admin.go**: 解析 `category_id` 查询参数、加载分类列表传入模板、分页链接保留分类参数
- **admin_helpers_test.go**: 测试覆盖参数化查询和非法值边界
- **templates/admin/admin_posts.html**: 筛选栏布局重排，新增分类下拉框，保持选中状态

## 0.1.1 - 2026-06-12

### Added

- **Mermaid 图表渲染**: Mermaid.js v11 CDN 集成，支持在文章中使用 ` ```mermaid ` 代码块渲染流程图、时序图、类图等。Goldmark 渲染后客户端自动将 `<pre><code class=\"language-mermaid\">` 转换为 `<pre class=\"mermaid\">` 并初始化渲染。支持亮色/暗色主题自适应（`data-theme` 属性联动）。仅在 `post.html` 加载 CDN，与 KaTeX 模式一致。

## 0.1.0 - 2026-06-12

### Added

- **公共 `skin.go` 模块**: `SkinConfig` 结构体、3 个校验函数（`sanitizeThemeName`、`sanitizeSkinColor`、`sanitizeSkinLength`）、3 个正则表达式从 `main.go` 和 `admin.go` 合并到公共文件 `skin.go`，消除重复定义。
- **`_skin-vars.html` 模板片段**: 前台 4 个页面模板（`post.html`、`index.html`、`error.html`、`category_password.html`）中重复的 CSS 变量块提取为公共片段，统一通过 `{{template "_skin-vars.html" .}}` 引用。
- **构建脚本更新**: `Dockerfile` 和 `build.sh` 的编译命令中追加 `./skin.go`，确保两个 binary 均包含新模块。

### Changed

- **皮肤 CSS 变量仅作用于亮色模式**: 移除原 `:root[data-theme='dark']` 中应用亮色配置值的问题代码，暗色模式下回退到 `style.css` 的静态暗色默认值。得益于 CSS 特异性差异（`:root[data-theme='dark']` 特异性 0-1-0 > `:root` 的 0-0-1），style.css 中的暗色值正确覆盖动态亮色注入。

### Security

- **颜色值逐个校验注入**: 每个颜色配置项经 `sanitizeSkinColor()` 正则验证（支持 `#RGB`、`#RRGGBB`、`#RRGGBBAA`、`rgb()`、`rgba()`、`hsl()`、`hsla()`、合法 CSS 变量引用），非法值回退为默认值。不再对整段 CSS 使用 `template.CSS()`。

### Removed

- `main.go` 和 `admin.go` 中各删除 23 行重复的 `SkinConfig` 结构体定义 + 3 个 sanitize 函数定义（共 −118 行 Go 代码）。
- 4 个模板文件中删除重复的 `<style>:root, :root[data-theme='dark'] { ... }</style>` 块（共 −95 行模板代码）。

## 0.0.12 - 2026-06-12

### Fixed

- **CSS 中 `rgba()` 默认值被 Go html/template 替换为 `ZgotmplZ`**: Go 的 `html/template` 包在 `<style>` 上下文中对 CSS 值做安全过滤，`rgba(r,g,b,a)` 写法被判定为"不安全 CSS 值"，3 个 CSS 变量（`--header-bg`、`--theme-btn-hover-bg`、`--theme-btn-active-bg`）渲染为 `ZgotmplZ`，导致 `:root {}` 规则块被部分浏览器（尤其移动端 WebView）丢弃，CSS 内容泄漏到页面正文显示。已将 `rgba()` 默认值替换为等效的 8 位 hex 色值（`#RRGGBBAA`），Go CSS 安全过滤器接受 hex 格式。涉及文件：`main.go`、`admin.go`、`usr/themes/default/style.css`。

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


