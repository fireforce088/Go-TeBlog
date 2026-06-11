# Bug Report: 数据库路径硬编码 + 管理工具无法独立运行

> 发现日期: 2026-06-11
> 报告人: Hermes Agent
> 优先级: 高（影响密码重置、数据库恢复等运维操作）

---

## Bug 1: 数据库路径硬编码，无 `--db` 参数

### 问题描述

`main.go` 和 `admin.go` 中数据库路径均硬编码为 `./blog.sqlite`：

```go
// main.go 和 admin.go
db, err := sql.Open("sqlite", "./blog.sqlite")
```

且函数开头有：
```go
exeDir := filepath.Dir(exePath)
os.Chdir(exeDir)
```

这意味着数据库路径是相对于**二进制文件所在目录**的，不是当前工作目录。在 Docker 容器内通过 entrypoint 创建了符号链接 `/app/blog.sqlite -> /data/blog.sqlite`，所以正常工作。但**任何在容器外独立运行二进制文件的操作**都会使用错误的数据库。

### 复现步骤

```bash
# 在 dev 目录外运行 admin_app --reset-password
cd /root && /vol1/1000/dev/Go-TeBlog-dev/admin_app --reset-password --reset-user=admin --reset-pass=newpass

# admin_app 打开的是 /vol1/1000/dev/Go-TeBlog-dev/blog.sqlite，不是 /data/blog.sqlite
# 密码写到了错误的库，真正的博客库不受影响
```

### 影响

- 密码重置写错数据库
- 备份恢复操作时容易混淆
- 运维脚本必须 cd 到正确目录才能工作

### 修复建议

添加 `--db` 或 `--data-dir` 参数：

```go
dbPath := flag.String("db", "./blog.sqlite", "Database file path")
db, err := sql.Open("sqlite", *dbPath)
```

---

## Bug 2: blog_app (main.go) 不支持 `--init-user` / `--reset-password` 等管理参数

### 问题描述

管理用参数（`--init-user`、`--init-pass`、`--reset-password`、`--reset-user`、`--reset-pass`）**只在 `admin.go` 中定义**，`main.go`（blog_app 的入口）中没有。

当我们需要重置密码时，正确的做法是运行 `admin_app`（端口 8191），而不是 `blog_app`（端口 8190）。但：
- Docker 容器暴露了 blog_app 端口（8190），admin_app 端口（8191）未暴露
- 运维习惯容易跑错二进制文件
- 跑错后 blog_app 只报 "Server starting on :8190" + 端口冲突，无任何提示

### 复现步骤

```bash
# 错误的操作（blog_app 不支持这些参数）
docker exec go-teblog /app/blog_app --reset-password --reset-user=admin --reset-pass=newpass

# 输出: Server starting on :8190 → 端口冲突 → 静默失败
# 没有 "unrecognized flag" 错误，没有友好提示
```

### 影响

- 运维人员跑错命令不知情
- 浪费调试时间

### 修复建议

方案 A：在 `main.go` 中也添加这些参数，转发到数据库操作（推荐）。
方案 B：在 `admin_app` 中检测并提示 "请使用 admin_app 而不是 blog_app"。
方案 C：至少打印友好提示 "blog_app 不支持管理参数，请使用 admin_app"。

---

## Bug 3: blog_app 收到了 `--reset` 相关的不明 flag 时无任何提示

### 问题描述

Go 的 `flag` 包默认遇到未定义的 flag 不会报错——它会静默忽略。当 blog_app 收到它不认识的 `--reset-password` 时，既不报错也不提示，直接尝试启动 HTTP 服务。

### 影响

- 运维误操作零反馈
- 需要排查日志才知道

### 修复建议

使用 `flag.Parse()` 后检查 `flag.NArg()` 或定义明确的 flag set。或者简单的方式：在 server 启动前检查是否有未预期的 flag。

---

## Bug 4: entrypoint 脚本只在首次启动时初始化密码，无运行时密码重置机制

### 问题描述

`docker-entrypoint.sh` 第 25-27 行：

```bash
if [ ! -f "$DATA_DIR/blog.sqlite" ] && [ -n "$INIT_ADMIN_PASSWORD" ]; then
  /app/admin_app --init-user="$INIT_ADMIN_USER" --init-pass="$INIT_ADMIN_PASSWORD"
fi
```

密码初始化只在数据库首次创建时执行。如果用户需要重置密码，有几种选择：
1. 删库重建 → 丢失文章
2. 手动改 SQLite → 需要 bcrypt hash，没有现成工具
3. 在容器内跑 admin_app → 但端口冲突跑不起来

### 影响

- 用户忘记密码后需要手动 hash + SQL 注入
- 操作门槛高

### 修复建议

在 entrypoint 中提供重置密码脚本（通过环境变量触发）：
```bash
# docker run -e RESET_ADMIN_PASSWORD=newpassword ...
if [ -n "${RESET_ADMIN_PASSWORD:-}" ]; then
  /app/admin_app --reset-password --reset-user="$INIT_ADMIN_USER" --reset-pass="$RESET_ADMIN_PASSWORD"
fi
```

或者暴露 admin_app 端口（8191），让外部可以直接调用。

---

## Bug 5: SQLite WAL 模式导致备份文件不完整

### 问题描述

博客使用 SQLite WAL（Write-Ahead Logging）模式。写入操作先写到 `.sqlite-wal` 文件，再异步合并到主文件。当直接 `cp blog.sqlite` 备份时，WAL 中的最新数据未包含在内。

### 复现步骤

```bash
# 用户执行了写入后
cp /data/blog.sqlite /backup/blog.sqlite   # ← WAL 内容未包含
```

### 影响

- 常规 `cp` 备份可能丢失最近写入
- 数据库文件大小和 WAL 文件大小可能差异很大
- 需要先执行 `PRAGMA wal_checkpoint(TRUNCATE)` 再备份

### 修复建议

1. 在 entrypoint 中添加定时 `PRAGMA wal_checkpoint` 的定时任务
2. 提供备份脚本 `backup.sh`，序列化执行 checkpoint → backup → compress
3. 或者在 `main.go` 中启动时 `PRAGMA wal_checkpoint`
