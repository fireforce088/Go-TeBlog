# Go-TeBlog 旅游攻略自动配图脚本

`scripts/travel_image_picker.py` — 为 Markdown 格式的旅行攻略自动匹配高置信度图片，不修改原文件，不改 Go-TeBlog 主程序。

## 快速开始

```bash
cd /vol1/1000/dev/Go-TeBlog-dev

# 最简单的用法（仅 Wikimedia Commons）
python3 scripts/travel_image_picker.py 庐山攻略.md

# 带配置文件
python3 scripts/travel_image_picker.py 庐山攻略.md --config config/travel-image-sources.json

# 详细输出
python3 scripts/travel_image_picker.py 庐山攻略.md -v
```

## 输出文件

对输入的 `文章.md`，生成三个文件：

| 文件 | 说明 |
|:-----|:------|
| `文章.with-images.md` | 带配图的 Markdown（置信度 >= 70 的图片自动插入） |
| `文章.image_candidates.json` | 所有候选图片的完整信息（含 50-70 分的低置信度候选） |
| `文章.image_report.md` | 配图报告 Markdown（统计 + 每张图的来源/作者/许可） |

> ⚠️ 脚本**不会修改**原文件。所有输出写在新文件中。

## 配图逻辑

### 置信度评分

| 分数 | 行为 |
|:----:|:------|
| >= 70 | 自动插入 Markdown（格式见下方） |
| 50-69 | 仅写入 JSON 候选文件，供人工挑选 |
| < 50 | 丢弃。段落标记为 `<!-- TODO_IMAGE -->` |

### 自动插入的图片格式

```markdown
![花径 - Lushan Huajing.jpg](https://upload.wikimedia.org/wikipedia/commons/...)
> 图源：https://commons.wikimedia.org/...，作者：Zhang San，许可：CC BY-SA 4.0
```

### 需人工确认的段落

当找不到高置信度图片时，在 Markdown 中保留：
```markdown
<!-- TODO_IMAGE: 需要人工确认：大天池 -->
```

## 配置

复制示例配置文件：

```bash
cp config/travel-image-sources.example.json config/travel-image-sources.json
# 编辑 config/travel-image-sources.json 按需调整
```

### 图片来源

| 源 | 需要 API Key | 许可 | 默认 |
|:----|:------------:|:----:|:----:|
| Wikimedia Commons | 否 | 各文件独立（CC/PD 为主） | ✅ 启用 |
| Unsplash | 是 | Unsplash License（免费商用） | ❌ 关闭 |
| Pexels | 是 | Pexels License（免费商用） | ❌ 关闭 |

### 置信度阈值

```json
{
  "confidence_thresholds": {
    "auto_insert": 70,
    "candidate_min": 50
  }
}
```

## 测试用例：庐山攻略

本项目提供的测试案例对应 Obsidian 库中的 _庐山 3天2晚最终行程计划（2026年6月24日—26日）_：

```bash
# 复制到项目目录
cp "/mnt/win-obsidian/020-Area/旅游攻略/庐山3天2晚最终行程计划_2026-06-24至06-26.md" /tmp/lushan-test.md

# 运行配图脚本
python3 scripts/travel_image_picker.py /tmp/lushan-test.md -v

# 查看输出
ls -la /tmp/lushan-test.with-*
cat /tmp/lushan-test.image_report.md
```

### 庐山已知景点（脚本内置）

处理庐山攻略时，脚本能自动识别以下景点：

- **西线**：花径、如琴湖、锦绣谷、仙人洞、大天池
- **东线**：含鄱口、五老峰、三叠泉
- **中线**：美庐别墅、庐山会议旧址、芦林湖、庐山博物馆
- **交通**：牯岭镇、庐山索道、九江站、庐山站
- **城市**：庐山、厦门、九江

## 验收对照

| # | 要求 | 实现 |
|:--:|:-----|:----:|
| 1 | 具体景点段落不使用只匹配城市名的图片 | ✅ 搜索 query 优先使用景点全称 |
| 2 | 每张自动插入图片有 source_url 和 license | ✅ 输出格式强制包含 |
| 3 | 低置信度图片不进入正文 | ✅ < 70 分不插入 |
| 4 | 脚本失败不破坏原 Markdown | ✅ 只读打开原文件，只写新文件 |
| 5 | 提供庐山攻略测试用例 | ✅ 本文档包含 |

## 扩展

### 添加新的图片源

继承 `ImageProvider` 基类并实现 `search()` 方法：

```python
class MyProvider(ImageProvider):
    name = "My Image Source"

    def search(self, query, place, max_results=5):
        # 返回 list[ImageCandidate]
        pass
```

然后在 `config/travel-image-sources.json` 中添加源配置。
