# 博客问题诊断报告

## 发现问题

### 问题 A — 外部图片域名被 strip

**现象**: 所有外链图片（包括 Wikimedia Commons、Unsplash 等）在博客渲染后域名被去除。
- DB 中: `![alt](https://upload.wikimedia.org/wikipedia/commons/e/ef/xxx.jpg)`
- 渲染后: `<img src="/wikipedia/commons/e/ef/xxx.jpg">` (域名被删)

**根因**: `main.go` 中的 `fixAttachmentLinks()` 函数（第 998-1022 行）在 Goldmark 渲染后处理 HTML 输出。它只对 `img.w-tx.top` 做了例外跳转，对外部其他域名的图片所有 URL 都被代码处理了（注意：观察发现路径不以 `/usr/` 开头时应 `return match` 保持原样，但实际输出了 strip 域名后的路径）。

**涉及范围**: 所有使用外链图片的文章（~16 篇使用 Unsplash + 3 篇使用 Wikimedia Commons 的旅游攻略）

**验证方法**: 
```bash
# 对比 DB 原文 vs 最终渲染 HTML
docker exec go-teblog sqlite3 /data/blog.sqlite "SELECT substr(text, 200, 100) FROM typecho_contents WHERE cid=110;"
# 输出应含 upload.wikimedia.org 完整域名
# 然后 curl 渲染页，grep 确认域名还在
```

### 问题 B — 批量配图文章的 HTML 代码裸露

**现象**: 后台文章编辑器可以看到裸的 HTML 标签（`<p align="center">`, `<img>`, `style="color:#666;font-size:13px;"` 等）

**根因**: 之前的批量配图操作在 16 篇文章正文中插入了 HTML 包裹的图片代码，违反博客纯 Markdown 内容规范。

**影响文章**: 16 篇（CID: 94-109 的旅游攻略 + CID 1 和 20）

**转换示例（每篇文章都要处理）**:
```
转换前:
<p align="center"><img src="https://images.unsplash.com/photo-XXXXX?auto=format&fit=crop&w=800&q=80" alt="描述"></p>
<p align="center" style="color:#666;font-size:13px;">📷 图片说明</p>

转换后:
![描述](https://images.unsplash.com/photo-XXXXX?auto=format&fit=crop&w=800&q=80)

📷 图片说明
```

### 问题 C — Unsplash 图片源全部 404

**现象**: `images.unsplash.com` 上所有 photo ID 都已返回 404

**范围**: 全部 16 篇受影响文章的配图源已死，即使格式转换正确也无法显示图片

**处理策略**: 
1. 先转换 HTML→纯 Markdown（修复格式问题）
2. Unsplash 死链保留原 URL 不动（暂无替换源时比断链好）

## 需要 Codex CLI 做的事情

1. **修复 `fixAttachmentLinks` 逻辑**: 将所有外部域名（不限于 `img.w-tx.top`）的绝对 URL 都保持原样，不 strip 域名。修改 `main.go` 第 996-1022 行的 `fixAttachmentLinks` 函数。

2. **编写批量内容修复脚本**: 编写一个 Python 脚本 `scripts/fix-html-images-to-markdown.py`，将 16 篇文章中的 HTML 图片包裹（`<p align="center"><img ...>`）和说明文字（`<p align="center" style="...">📷 ...</p>`）转换为纯 Markdown 格式。

3. **验证**:
   - 验证 `fixAttachmentLinks` 不修改任何外部 URL
   - 运行 Python 脚本更新数据库
   - 重启容器后验证文章 110 和 108 的图片 URL 完整

## 文件位置
- 源码: `/vol1/1000/dev/Go-TeBlog-dev/main.go` (第 996-1022 行)
- 数据库: `/vol1/1000/dev/Go-TeBlog-dev/data/blog.sqlite`
- 脚本目录: `/vol1/1000/dev/Go-TeBlog-dev/scripts/`
- 工作目录: `/vol1/1000/dev/Go-TeBlog-dev/`

## Go 版本
使用 `/usr/local/go/bin/go` (1.25)，不要用系统自带的 `/usr/bin/go` (1.19)
