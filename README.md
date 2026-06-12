# ⚡ Go-TeBlog

<div align="center">

**一个基于 Go 语言的极速博客系统 — Typecho 兼容，开箱即用**

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?style=flat&logo=docker)](https://docker.com)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen)

</div>

---

Go-TeBlog 是一个轻量级、高性能的博客系统，使用 Go 语言从零实现。它兼容 Typecho 的数据库结构和附件目录，可直接作为 Typecho 的平滑升级替代品，也适合作为独立博客系统使用。

## 核心特性

- **极速性能** — Go 原生编译，无解释器开销，单二进制部署，内存常驻仅 15MB
- **Typecho 完全兼容** — 直接使用 Typecho 的 SQLite 数据库和附件目录，零迁移成本
- **SQLite 存储** — 无需 MySQL/PostgreSQL，数据文件即数据库，备份就是一个文件
- **Markdown 写作** — 原生支持 Markdown，拖拽/粘贴上传图片，自动关联附件
- **评论系统** — 支持审核机制、频率限制、黑名单、嵌套评论
- **分类管理** — 支持排序、首页显示控制、整类下线隐藏、分类密码访问
- **LaTeX 数学公式** — 内置 KaTeX 渲染，行内 $...$ 和块级 $$...$$ 均支持
- **Mermaid 图表** — 支持流程图、时序图、类图等代码块渲染
- **访客统计** — 基于 Beacon 的真人访客统计，区分机器人流量
- **多用户后台** — 支持用户管理、角色区分、访客只读模式
- **一键备份** — 后台直接备份数据库和附件，支持 VACUUM
- **SEO 友好** — 内置 sitemap.xml，完整的归档、分类、搜索页面
- **Docker 部署** — 一行命令启动，支持 MinIO 对象存储扩展

## 快速开始

### 方式一：Docker 部署（推荐）

```bash
# 克隆项目
git clone https://github.com/fireforce088/Go-TeBlog.git
cd Go-TeBlog

# 启动服务
docker compose up -d

# 查看初始化密码
docker logs go-teblog 2>&1 | grep "初始管理员密码"
```

访问 `http://localhost:8190/blog` 查看前台，`http://localhost:8190/admin` 进入后台。

> 首次启动时，系统会自动创建管理员账号并打印随机密码到日志中。登录后请在"用户管理"中修改密码。

### 方式二：二进制部署

```bash
# 环境要求：Go 1.24+ 或直接使用预编译二进制
sudo bash build.sh
```

## Docker 生产部署

### 基础部署

```yaml
services:
  go-teblog:
    image: go-teblog:v0.1.3
    container_name: go-teblog
    restart: unless-stopped
    ports:
      - "8190:8190"
    environment:
      TZ: Asia/Shanghai
      GIN_MODE: release
    volumes:
      - ./data:/data
```

### 使用 MinIO 对象存储（可选）

Go-TeBlog 支持将上传的图片和附件同步到 MinIO 对象存储，适合多实例部署或需要 CDN 分发场景。

```yaml
services:
  go-teblog:
    image: go-teblog:v0.1.3
    ports:
      - "8190:8190"
    environment:
      GIN_MODE: release
      MINIO_ENDPOINT: http://minio:9000
      MINIO_ACCESS_KEY: your-access-key
      MINIO_SECRET_KEY: your-secret-key
      MINIO_BUCKET: blog-images
      MINIO_PUBLIC_URL: https://img.your-domain.com
    volumes:
      - ./data:/data
```

### 反向代理 + HTTPS

推荐使用 Caddy、Nginx 或 Lucky 将请求反代到 `:8190` 端口。Go-TeBlog 本身不处理 TLS，建议通过反向代理配置 HTTPS。

## 从 Typecho 迁移

Go-TeBlog 完全兼容 Typecho 的 SQLite 数据库结构，迁移步骤极简：

### 场景 A：原 Typecho 使用 SQLite

1. 停止旧站写入
2. 将旧站 SQLite 数据库复制到项目主目录，重命名为 `blog.sqlite`
3. 将旧站附件目录 `usr/uploads` 复制到项目主目录下的 `usr/uploads`
4. 启动 Go-TeBlog

### 场景 B：原 Typecho 使用 MySQL/PostgreSQL

项目 `tools/` 目录提供迁移脚本，可将 MySQL/PostgreSQL 数据库无损转换为 SQLite：

```bash
cd tools
go run tosqlite.go
```

按提示选择源库类型并填写连接信息即可。

## 功能预览

### 前台
- 清爽的文章列表页（首页/分类/归档）
- 文章详情页（支持 LaTeX / Mermaid / 目录导航）
- 全文搜索
- 评论互动

### 后台
- **仪表盘** — 访客统计趋势、服务器运行参数
- **文章管理** — 支持 Markdown 编辑、图片上传、分类标签、按分类/状态/关键词筛选
- **评论管理** — 审核/隐藏/删除，频率限制与黑名单
- **分类管理** — 排序、首页显示控制、密码保护、整类下线
- **附件管理** — 按文章检索附件，一键删除引用
- **用户管理** — 新增/编辑/删除/角色管理
- **系统设置** — 站点信息、SEO、评论审核开关等全局配置
- **数据备份** — 一键备份数据库和上传文件

## 核心技术栈

| 层 | 技术 |
|---|---|
| 后端框架 | [Gin](https://github.com/gin-gonic/gin) |
| 数据库 | SQLite（原生 CGo 驱动） |
| Markdown | [Goldmark](https://github.com/yuin/goldmark) (CommonMark) |
| 数学公式 | KaTeX（CDN 加载） |
| 图表渲染 | Mermaid.js（CDN 加载） |
| 对象存储 | MinIO SDK |

## 项目结构

```
Go-TeBlog/
├── main.go                 # 前台路由/渲染 (~2900 行)
├── admin.go                # 后台管理逻辑 (~3300 行)
├── admin_helpers.go        # 后台辅助函数
├── admin_helpers_test.go   # 辅助函数测试
├── admin_storage.go        # MinIO 存储
├── editor_normalize.go     # HTML→Markdown 转换
├── editor_normalize_test.go
├── skin.go                 # 皮肤变量公共模块
├── templates/
│   ├── admin/              # 后台模板 (14 文件)
│   └── frontend/           # 前台模板 (5 文件)
├── usr/themes/default/     # 前台皮肤 (CSS/JS)
├── Dockerfile              # Docker 镜像构建
├── docker-compose.yml      # Docker Compose 配置
└── build.sh                # 一键部署脚本
```

## 设计理念

- **极简** — 核心代码集中在少数文件中，降低上下文复杂度
- **兼容** — 完全兼容 Typecho 数据格式，用户数据不被锁定
- **轻量** — 单二进制 + SQLite，可运行在任何廉价的 VPS 甚至树莓派上
- **AI 驱动开发** — 本项目 100% 由 AI 编写代码，人类仅提供需求描述和方向指导。所有安全相关代码经过人工审核。

## 致谢

- [Typecho](https://typecho.org/) — 设计灵感与数据兼容
- [Gin](https://github.com/gin-gonic/gin)、[Goldmark](https://github.com/yuin/goldmark) — 优秀的开源基础库
- [Claude](https://www.anthropic.com/claude)、[Gemini](https://deepmind.google/technologies/gemini/)、[Codex CLI](https://github.com/openai/codex) — AI 编码助手
- 所有贡献者和使用者

## 开源协议

[MIT License](LICENSE)
