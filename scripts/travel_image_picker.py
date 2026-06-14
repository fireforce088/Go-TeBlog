#!/usr/bin/env python3
"""
Go-TeBlog 旅游攻略自动配图预处理脚本

功能：
  - 读取 Markdown 旅行攻略文件
  - 抽取地点实体（城市/景点/地标）
  - 通过可插拔图片源为每个段落寻找高置信度图片
  - 输出带配图的 Markdown + 候选报告

用法：
  python3 scripts/travel_image_picker.py path/to/article.md [--config config.json]

不修改原文件，不更改 Go-TeBlog 主程序。
"""

import argparse
import hashlib
import json
import os
import re
import sys
import traceback
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional
from urllib.parse import quote, urlencode

import requests

# ─── 数据结构 ───────────────────────────────────────────────


@dataclass
class ImageCandidate:
    image_url: str
    source_url: str
    title: str
    author: str
    license: str
    matched_place: str
    confidence_score: int  # 0-100
    reason: str


@dataclass
class ParagraphInfo:
    heading_level: int  # 0=title, 1=h1, 2=h2, ...
    heading_text: str
    paragraph_text: str
    line_start: int  # 0-indexed
    line_end: int    # exclusive
    images: list[ImageCandidate] = field(default_factory=list)
    chosen_image: Optional[ImageCandidate] = None


# ─── 图片源提供者 基类 ────────────────────────────────────


class ImageProvider:
    """可插拔图片源基类。每个子类实现 search() 返回候选图片列表。"""
    name: str = "base"

    def search(self, query: str, place: str, max_results: int = 5) -> list[ImageCandidate]:
        raise NotImplementedError


class WikimediaProvider(ImageProvider):
    """Wikimedia Commons API 图片源（免费、免密钥、许可明确）。"""
    name = "Wikimedia Commons"

    COMMONS_API = "https://commons.wikimedia.org/w/api.php"
    HEADERS = {
        "User-Agent": "Go-TeBlog-TravelImagePicker/1.0 (https://blog.w-tx.cn; helpcn@126.com) Python/3.11",
    }

    def _get(self, params: dict) -> requests.Response:
        return requests.get(self.COMMONS_API, params=params, headers=self.HEADERS, timeout=15)

    def search(self, query: str, place: str, max_results: int = 5) -> list[ImageCandidate]:
        candidates = []
        try:
            # Step 1: search Commons for the query
            params = {
                "action": "query",
                "list": "search",
                "srsearch": query + " " + place,
                "srnamespace": "6",  # File namespace
                "format": "json",
                "srlimit": min(max_results * 2, 50),
                "srprop": "snippet|titlesnippet",
            }
            resp = self._get(params)
            resp.raise_for_status()
            data = resp.json()

            pages = data.get("query", {}).get("search", [])
            if not pages:
                # Fallback: broader search with just the place name
                params["srsearch"] = place
                resp = self._get(params)
                resp.raise_for_status()
                data = resp.json()
                pages = data.get("query", {}).get("search", [])

            for page in pages[:min(max_results, 2)]:
                title = page.get("title", "")
                if not title or ":" not in title:
                    continue
                # Extract filename after "File:" prefix
                filename = title.split(":", 1)[1] if ":" in title else title
                snippet = page.get("snippet", "")
                # Skip icons, maps, logos (small files / vector)
                if any(x in filename.lower() for x in [".svg", ".ogg", ".ogv", ".pdf"]):
                    continue

                # Step 2: get image URL + metadata (one additional API call per candidate)
                img_info = self._get_image_info(title)
                if not img_info:
                    continue

                image_url = img_info.get("url", "")
                author = img_info.get("author", "Unknown")
                license_name = img_info.get("license", "Unknown")
                desc_url = img_info.get("description_url", "")

                # Step 3: calculate confidence
                score, reason = self._score_match(query, place, filename, snippet)
                if score < 50:
                    continue

                candidates.append(ImageCandidate(
                    image_url=image_url,
                    source_url=desc_url,
                    title=filename,
                    author=author,
                    license=license_name,
                    matched_place=place,
                    confidence_score=score,
                    reason=reason,
                ))

            candidates.sort(key=lambda c: c.confidence_score, reverse=True)
            return candidates

        except requests.RequestException as e:
            print(f"  [WARN] Wikimedia API error: {e}", file=sys.stderr)
            return []
        except Exception as e:
            print(f"  [WARN] Wikimedia parse error: {e}", file=sys.stderr)
            return []

    def _get_image_info(self, file_title: str) -> dict | None:
        """获取图片 URL、作者、许可信息。"""
        params = {
            "action": "query",
            "titles": file_title,
            "prop": "imageinfo|info",
            "iiprop": "url|extmetadata|user",
            "format": "json",
        }
        try:
            resp = self._get(params)
            resp.raise_for_status()
            data = resp.json()
            pages = data.get("query", {}).get("pages", {})
            for pid, info in pages.items():
                if pid == "-1":
                    continue
                iinfo = info.get("imageinfo", [{}])[0]
                extmeta = iinfo.get("extmetadata", {})

                # Parse license
                license_name = "Unknown"
                for lic_field in ["LicenseShortName", "License", "UsageTerms"]:
                    lic_data = extmeta.get(lic_field, {})
                    val = lic_data.get("value", "")
                    if val:
                        license_name = val
                        break

                # Parse author
                author = "Unknown"
                for auth_field in ["Artist", "Credit", "Author"]:
                    auth_data = extmeta.get(auth_field, {})
                    val = auth_data.get("value", "")
                    if val:
                        # Strip HTML tags from author string
                        author = re.sub(r'<[^>]+>', '', val).strip()
                        if author:
                            break

                return {
                    "url": iinfo.get("url", ""),
                    "author": author[:200],
                    "license": license_name[:100],
                    "description_url": iinfo.get("descriptionurl", ""),
                }
        except Exception:
            return None

    def _score_match(self, query: str, place: str, filename: str, snippet: str) -> tuple[int, str]:
        """计算置信度。基于文件名与 query/place 的包含关系。"""
        score = 50
        reasons = []
        text_lower = (filename + " " + snippet).lower()
        query_lower = query.lower()
        place_lower = place.lower()

        # Keywords in query
        query_keywords = set(re.findall(r'[\w\u4e00-\u9fff]+', query_lower))
        place_keywords = set(re.findall(r'[\w\u4e00-\u9fff]+', place_lower))

        # Check if query keywords match in filename
        matched_keywords = sum(1 for kw in query_keywords if kw in text_lower and len(kw) > 1)
        if matched_keywords >= 2:
            score += 25
            reasons.append(f"文件名含{matched_keywords}个query关键词")
        elif matched_keywords == 1:
            score += 15
            reasons.append("文件名含1个query关键词")

        # Check if place name matches
        place_matched = sum(1 for kw in place_keywords if kw in text_lower and len(kw) > 1)
        if place_matched >= 1:
            score += 10
            reasons.append("文件名含地点关键词")

        # Negative: if filename is too generic
        generic_words = {"view", "photo", "image", "picture", "风景", "照片", "图"}
        if any(gw in filename.lower() for gw in generic_words):
            if matched_keywords == 0:
                score -= 10
                reasons.append("文件名太笼统")
            else:
                score -= 5
                reasons.append("文件名部分笼统")

        score = max(0, min(100, score))
        final_reason = "; ".join(reasons) if reasons else f"基础匹配(score={score})"
        return score, final_reason


class UnsplashProvider(ImageProvider):
    """Unsplash API 图片源（需要 API Key，可选）。"""
    name = "Unsplash"

    API_URL = "https://api.unsplash.com/search/photos"

    def __init__(self, access_key: str = ""):
        self.access_key = access_key

    def search(self, query: str, place: str, max_results: int = 5) -> list[ImageCandidate]:
        if not self.access_key:
            return []
        try:
            search_term = f"{query} {place}" if query else place
            params = {
                "query": search_term,
                "per_page": min(max_results * 2, 30),
                "orientation": "landscape",
            }
            headers = {"Authorization": f"Client-ID {self.access_key}"}
            resp = requests.get(self.API_URL, params=params, headers=headers, timeout=10)
            resp.raise_for_status()
            data = resp.json()

            candidates = []
            for photo in data.get("results", [])[:max_results]:
                image_url = photo.get("urls", {}).get("raw", "")
                if not image_url:
                    continue
                user = photo.get("user", {})
                author = user.get("name", "Unknown")
                author_url = user.get("links", {}).get("html", "")
                candidates.append(ImageCandidate(
                    image_url=f"{image_url}&w=1200",
                    source_url=photo.get("links", {}).get("html", ""),
                    title=photo.get("description", "") or photo.get("alt_description", "") or query,
                    author=f"{author} ({author_url})",
                    license="Unsplash License (free for commercial use)",
                    matched_place=place,
                    confidence_score=75,
                    reason="Unsplash 高质量图片",
                ))
            return candidates
        except Exception as e:
            print(f"  [WARN] Unsplash API error: {e}", file=sys.stderr)
            return []


class PexelsProvider(ImageProvider):
    """Pexels API 图片源（需要 API Key，可选）。"""
    name = "Pexels"

    API_URL = "https://api.pexels.com/v1/search"

    def __init__(self, api_key: str = ""):
        self.api_key = api_key

    def search(self, query: str, place: str, max_results: int = 5) -> list[ImageCandidate]:
        if not self.api_key:
            return []
        try:
            search_term = f"{query} {place}" if query else place
            headers = {"Authorization": self.api_key}
            params = {
                "query": search_term,
                "per_page": min(max_results * 2, 40),
                "orientation": "landscape",
            }
            resp = requests.get(self.API_URL, params=params, headers=headers, timeout=10)
            resp.raise_for_status()
            data = resp.json()

            candidates = []
            for photo in data.get("photos", [])[:max_results]:
                image_url = photo.get("src", {}).get("large", "")
                if not image_url:
                    continue
                photographer = photo.get("photographer", "Unknown")
                photographer_url = photo.get("photographer_url", "")
                candidates.append(ImageCandidate(
                    image_url=image_url,
                    source_url=photo.get("url", ""),
                    title=query,
                    author=f"{photographer} ({photographer_url})",
                    license="Pexels License (free for commercial use)",
                    matched_place=place,
                    confidence_score=75,
                    reason="Pexels 高质量免费图片",
                ))
            return candidates
        except Exception as e:
            print(f"  [WARN] Pexels API error: {e}", file=sys.stderr)
            return []


# ─── 地点抽取 ────────────────────────────────────────────────


# 常见中国城市名库（可扩展）
KNOWN_CITIES = {
    "北京", "上海", "广州", "深圳", "杭州", "南京", "苏州", "厦门", "成都",
    "重庆", "武汉", "西安", "长沙", "昆明", "丽江", "大理", "桂林", "海口",
    "三亚", "青岛", "大连", "天津", "宁波", "福州", "合肥", "南昌", "九江",
    "庐山", "黄山", "泰山", "华山", "峨眉山", "武夷山", "张家界",
    "贵阳", "拉萨", "哈尔滨", "长春", "沈阳", "呼和浩特", "乌鲁木齐",
    "兰州", "银川", "西宁", "郑州", "太原", "石家庄", "济南", "南宁",
}

# 常见景点/地标后缀词
SPOT_SUFFIXES = {
    "山", "湖", "江", "河", "海", "岛", "湾", "峰", "谷", "洞", "瀑",
    "寺", "庙", "塔", "桥", "楼", "阁", "园", "林", "宫", "殿",
    "口", "关", "城", "堡", "陵", "墓", "亭", "台", "崖",
    "滩", "泉", "溪", "潭", "池",
    "站", "机场", "码头", "广场", "街", "路", "道",
    "旧址", "故居", "博物馆", "公园", "景区", "保护区", "度假区",
}

# 知名景点（硬编码以提升匹配精度）
KNOWN_SPOTS: dict[str, set[str]] = {
    "庐山": {"含鄱口", "五老峰", "三叠泉", "花径", "如琴湖", "锦绣谷",
              "仙人洞", "大天池", "美庐别墅", "庐山会议旧址", "芦林湖",
              "庐山博物馆", "牯岭镇", "庐山索道"},
    "厦门": {"厦门北站", "鼓浪屿", "曾厝垵", "环岛路", "厦门大学", "南普陀寺"},
    "南京": {"中山陵", "明孝陵", "夫子庙", "秦淮河", "总统府", "南京博物院",
              "老门东", "玄武湖"},
    "杭州": {"西湖", "断桥", "雷峰塔", "灵隐寺", "宋城", "南宋御街", "河坊街"},
    "苏州": {"拙政园", "苏州博物馆", "平江路", "虎丘", "寒山寺", "山塘街",
              "金鸡湖"},
}


def extract_places_from_text(text: str, known_cities: set[str],
                              spot_suffixes: set[str],
                              known_spots: dict[str, set[str]]) -> dict[str, set[str]]:
    """从文本中提取城市和景点名称。返回 {city: {spot1, spot2, ...}}"""
    found: dict[str, set[str]] = {}

    # Find cities from known list
    for city in known_cities:
        if city in text:
            found.setdefault(city, set())

    # Find spots from known_spots
    for city, spots in known_spots.items():
        if city not in found and city in text:
            found.setdefault(city, set())
        for spot in spots:
            if spot in text:
                if city not in found:
                    # Auto-detect city from known spots
                    for c in known_cities:
                        if c in text:
                            city = c
                            found.setdefault(city, set())
                            break
                    else:
                        found.setdefault(city, set())
                found[city].add(spot)

    # If no city found via known list, infer from context
    if not found:
        # Use first few lines to guess
        lines = text.split("\n")[:10]
        for line in lines:
            words = re.findall(r'[\u4e00-\u9fff]{2,4}(?:山|湖|江|岛|湾|峰|谷)', line)
            if words:
                generic_city = "目的地"
                found.setdefault(generic_city, set()).update(words)
                break

    return found


def extract_paragraphs(markdown_text: str) -> list[ParagraphInfo]:
    """将 Markdown 按标题分割为段落。"""
    lines = markdown_text.split("\n")
    paragraphs: list[ParagraphInfo] = []
    current_heading = ""
    current_level = 0
    current_lines: list[str] = []
    current_start = 0

    def flush():
        nonlocal current_lines, current_start
        text = "\n".join(current_lines).strip()
        if text or (current_heading and current_lines):
            para = ParagraphInfo(
                heading_level=current_level,
                heading_text=current_heading,
                paragraph_text=text,
                line_start=current_start,
                line_end=current_start + len(current_lines),
            )
            paragraphs.append(para)
        current_lines = []
        current_start = 0

    for i, line in enumerate(lines):
        stripped = line.strip()
        heading_match = re.match(r'^(#{1,6})\s+(.+)$', stripped)

        if heading_match:
            flush()
            current_level = len(heading_match.group(1))
            current_heading = heading_match.group(2).strip()
            current_start = i
        elif stripped:
            if not current_lines:
                current_start = i
            current_lines.append(stripped)
        else:
            # Empty line = paragraph boundary
            if current_lines:
                flush()

    # Final flush
    if current_lines or (current_heading and not paragraphs):
        flush()

    # First paragraph with # title → set level to 0
    if paragraphs and paragraphs[0].heading_level == 1:
        paragraphs[0].heading_level = 0  # Title

    return paragraphs


def generate_query(para: ParagraphInfo, found_places: dict[str, set[str]]) -> tuple[str, str]:
    """
    为段落生成搜索 query 和主要地点名称。
    策略：如果段落标题包含已知景点，用景点名；否则用城市名 + 段落关键词。
    """
    heading = para.heading_text
    text = para.paragraph_text
    combined = heading + " " + text

    # Check known spots in heading first
    for city, spots in found_places.items():
        for spot in sorted(spots, key=len, reverse=True):
            if spot in heading:
                # The heading directly mentions a known spot
                return spot, spot

    # Check known spots in paragraph text
    for city, spots in found_places.items():
        for spot in sorted(spots, key=len, reverse=True):
            if spot in combined:
                return f"{city} {spot}", spot

    # Fallback: use city + heading keywords
    if found_places:
        primary_city = max(found_places.keys(), key=lambda c: len(c))
        # Extract key nouns from heading (Chinese 2-4 char words)
        keywords = re.findall(r'[\u4e00-\u9fff]{2,6}', heading)
        if keywords:
            query = f"{primary_city} {' '.join(keywords[:3])}"
            return query, primary_city
        return primary_city, primary_city

    return heading, heading


# ─── 图片下载 ────────────────────────────────────────────────


def download_image(image_url: str, download_dir: str) -> tuple[str, str]:
    """下载远程图片到本地目录，返回 (local_path, local_url_path)。

    参数：
        image_url: 远程图片 URL（如 Wikimedia Commons 链接）
        download_dir: 下载目标目录（如 /vol1/1000/Docker/Go-Blog/blog-images/）

    返回：
        (local_file_path, local_url_path)
        示例: ("/data/blog-images/ab12cd34.jpg", "/blog-images/ab12cd34.jpg")

    异常时抛出。
    """
    os.makedirs(download_dir, exist_ok=True)

    # 用 URL 的 SHA256 前缀作为文件名，去重
    url_hash = hashlib.sha256(image_url.encode()).hexdigest()[:8]
    ext = os.path.splitext(image_url)[1] or ".jpg"
    filename = f"{url_hash}{ext}"
    local_path = os.path.join(download_dir, filename)

    # 已存在则跳过
    if os.path.exists(local_path):
        return local_path, f"/blog-images/{filename}"

    # 下载
    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
    }
    resp = requests.get(image_url, headers=headers, timeout=30, stream=True)
    resp.raise_for_status()

    with open(local_path, "wb") as f:
        for chunk in resp.iter_content(chunk_size=8192):
            f.write(chunk)

    return local_path, f"/blog-images/{filename}"


def download_chosen_images(paragraphs: list, download_dir: str, verbose: bool = False) -> int:
    """下载所有选中配图到本地目录，并更新 ImageCandidate 的 image_url 为本地路径。

    返回：成功下载/已存在的图片数量。
    """
    count = 0
    for para in paragraphs:
        if not para.chosen_image:
            continue
        try:
            local_path, local_url = download_image(para.chosen_image.image_url, download_dir)
            para.chosen_image.image_url = local_url
            count += 1
            if verbose:
                print(f"  [DOWNLOAD] {local_path}")
        except Exception as e:
            if verbose:
                print(f"  [WARN] 下载失败 {para.chosen_image.image_url[:60]}...: {e}")
            # 下载失败则保持远程 URL 不变
    return count


# ─── 主流程 ──────────────────────────────────────────────────


def load_config(config_path: str) -> dict:
    """加载配置文件。"""
    defaults = {
        "sources": {
            "wikimedia": {"enabled": True},
            "unsplash": {"enabled": False, "access_key": ""},
            "pexels": {"enabled": False, "api_key": ""},
        },
        "confidence_thresholds": {
            "auto_insert": 70,
            "candidate_min": 50,
        },
        "max_images_per_paragraph": 3,
        "request_timeout": 15,
    }
    if config_path and os.path.exists(config_path):
        with open(config_path, encoding="utf-8") as f:
            user_config = json.load(f)
        _deep_merge(defaults, user_config)
    return defaults


def _deep_merge(base: dict, override: dict) -> None:
    for key, val in override.items():
        if key in base and isinstance(base[key], dict) and isinstance(val, dict):
            _deep_merge(base[key], val)
        else:
            base[key] = val


def build_providers(config: dict) -> list[ImageProvider]:
    """根据配置构建图片源提供者列表。"""
    providers: list[ImageProvider] = []
    sources = config.get("sources", {})

    if sources.get("wikimedia", {}).get("enabled", True):
        providers.append(WikimediaProvider())

    us = sources.get("unsplash", {})
    if us.get("enabled") and us.get("access_key"):
        providers.append(UnsplashProvider(access_key=us["access_key"]))

    px = sources.get("pexels", {})
    if px.get("enabled") and px.get("api_key"):
        providers.append(PexelsProvider(api_key=px["api_key"]))

    return providers


def process_markdown(input_path: str, config: dict, download_dir: str = "") -> dict:
    """主处理函数。返回处理结果字典。"""
    with open(input_path, encoding="utf-8") as f:
        markdown_text = f.read()

    base_name = os.path.splitext(input_path)[0]
    output_md = base_name + ".with-images.md"
    output_json = base_name + ".image_candidates.json"
    output_report = base_name + ".image_report.md"

    # Step 1: Extract places
    found_places = extract_places_from_text(
        markdown_text, KNOWN_CITIES, SPOT_SUFFIXES, KNOWN_SPOTS
    )

    # Step 2: Split into paragraphs
    paragraphs = extract_paragraphs(markdown_text)

    # Step 3: Build providers
    providers = build_providers(config)
    auto_threshold = config.get("confidence_thresholds", {}).get("auto_insert", 70)
    cand_threshold = config.get("confidence_thresholds", {}).get("candidate_min", 50)
    max_per_para = config.get("max_images_per_paragraph", 3)

    # Step 4: For each paragraph, search for images
    # Only search for paragraphs that contain scenic spot names or are major sections
    total_searched = 0
    total_auto = 0
    total_candidate = 0
    total_todo = 0
    all_candidates: list[ImageCandidate] = []

    # Merge consecutive paragraphs under the same heading
    merged_paragraphs: list[ParagraphInfo] = []
    for para in paragraphs:
        if not para.heading_text and not para.paragraph_text:
            continue
        if para.paragraph_text.strip().startswith("```"):
            continue
        # Skip tables (lines starting with |)
        first_line = para.paragraph_text.strip().split("\n")[0] if para.paragraph_text.strip() else ""
        if first_line.startswith("|") and para.paragraph_text.count("|") > 3:
            continue
        # Skip checklists (all lines are checkboxes)
        if all(line.strip().startswith("- [") for line in para.paragraph_text.strip().split("\n") if line.strip()):
            continue
        # Skip short content (< 30 chars) that isn't a heading
        if len(para.paragraph_text.strip()) < 30 and not para.heading_text:
            continue

        merged_paragraphs.append(para)

    paragraphs = merged_paragraphs

    # Build a set of spot names for quick lookup
    all_known_spots: set[str] = set()
    for spots in found_places.values():
        all_known_spots.update(spots)
    # Also include spot suffixes matching
    spot_pattern = re.compile(r'[\u4e00-\u9fff]{2,6}(?:' + '|'.join(re.escape(s) for s in SPOT_SUFFIXES) + ')')

    # Limit total API calls with a budget
    MAX_API_CALLS = 10
    api_calls_made = 0

    for i, para in enumerate(paragraphs):

        query, place = generate_query(para, found_places)
        if not query or query == place and not place:
            continue

        # Limit API calls: only search when paragraph contains a known scenic spot or place name
        heading_has_spot = any(spot in para.heading_text for spot in all_known_spots)
        text_has_spot = bool(spot_pattern.search(para.paragraph_text))
        is_major_section = para.heading_level <= 2 and len(para.paragraph_text) > 100

        if not (heading_has_spot or text_has_spot or is_major_section):
            continue

        if api_calls_made >= MAX_API_CALLS:
            # Mark remaining paragraphs as TODO without searching
            total_todo += 1
            continue
        api_calls_made += 1

        total_searched += 1
        para_candidates: list[ImageCandidate] = []

        for provider in providers:
            results = provider.search(query, place, max_results=max_per_para)
            para_candidates.extend(results)

        # Deduplicate by image_url
        seen_urls = set()
        unique_candidates = []
        for c in sorted(para_candidates, key=lambda x: x.confidence_score, reverse=True):
            if c.image_url not in seen_urls:
                seen_urls.add(c.image_url)
                unique_candidates.append(c)

        para.images = unique_candidates[:max_per_para]
        all_candidates.extend(para.images)

        if unique_candidates and unique_candidates[0].confidence_score >= auto_threshold:
            para.chosen_image = unique_candidates[0]
            total_auto += 1
        elif unique_candidates and unique_candidates[0].confidence_score >= cand_threshold:
            total_candidate += 1
        else:
            total_todo += 1

    # Step 4.5: Download chosen images to local
    total_downloaded = 0
    if download_dir:
        total_downloaded = download_chosen_images(paragraphs, download_dir, verbose=False)

    # Step 5: Generate output markdown with images
    output_lines = []
    lines = markdown_text.split("\n")
    inserted_positions: set[int] = set()

    for para in paragraphs:
        if not para.chosen_image:
            continue
        img = para.chosen_image

        # Find insertion point: after paragraph text, before next heading
        insert_line = para.line_end
        while insert_line < len(lines) and lines[insert_line].strip() == "":
            insert_line += 1
        if insert_line in inserted_positions:
            insert_line = para.line_end
        inserted_positions.add(insert_line)

        img_markdown = (
            f"![{img.matched_place} - {img.title} {img.author}/{img.source_url.split('/')[2] if '//' in img.source_url else img.source_url}]"
            f"({img.image_url})\n"
            f"> 图源：{img.source_url}，作者：{img.author}，许可：{img.license}\n"
        )
        # Insert before the next heading (or at end of paragraph)
        # We'll track positions and insert later

    # Build output markdown
    output_lines = []
    for i, line in enumerate(lines):
        if i in inserted_positions:
            # Find which paragraph this position belongs to
            for para in paragraphs:
                if para.chosen_image and para.line_end <= i <= para.line_end + 1:
                    img = para.chosen_image
                    img_markdown = (
                        f"![{img.matched_place} - {img.title}]({img.image_url})\n"
                        f"> 图源：{img.source_url}，作者：{img.author}，许可：{img.license}\n"
                    )
                    output_lines.append(img_markdown)
                    break
        output_lines.append(line)

    output_text = "\n".join(output_lines)

    with open(output_md, "w", encoding="utf-8") as f:
        f.write(output_text)

    # Step 6: Output candidates JSON
    candidate_data = []
    for para in paragraphs:
        if not para.images and not para.chosen_image and para.heading_text:
            # No images found >= 50 → add TODO placeholder
            candidate_data.append({
                "heading": para.heading_text,
                "status": "todo_manual",
                "suggested_query": generate_query(para, found_places)[0],
                "images": [],
            })
        if para.images:
            candidate_data.append({
                "heading": para.heading_text,
                "status": "auto_inserted" if para.chosen_image else "candidate_only",
                "suggested_query": generate_query(para, found_places)[0],
                "images": [asdict(c) for c in para.images],
            })

    with open(output_json, "w", encoding="utf-8") as f:
        json.dump(candidate_data, f, ensure_ascii=False, indent=2)

    # Step 7: Output report
    report_lines = [
        f"# 图片配图报告：{os.path.basename(input_path)}",
        "",
        f"> 生成时间：{__import__('datetime').datetime.now().strftime('%Y-%m-%d %H:%M')}",
        "",
        "## 统计",
        "",
        f"| 项目 | 数量 |",
        f"|------|:----:|",
        f"| 总段落数 | {len(paragraphs)} |",
        f"| 已搜索配图的段落 | {total_searched} |",
        f"| 自动插入图片 | {total_auto} |",
        f"| 低置信度候选 | {total_candidate} |",
        f"| 需人工确认段落 | {total_todo} |",
        "",
        "## 已自动插入的图片",
        "",
    ]
    for para in paragraphs:
        if para.chosen_image:
            img = para.chosen_image
            report_lines.extend([
                f"### {para.heading_text}",
                "",
                f"![{img.matched_place}]({img.image_url})",
                "",
                f"| 字段 | 值 |",
                f"|------|-----|",
                f"| 匹配地点 | {img.matched_place} |",
                f"| 置信度 | {img.confidence_score}/100 |",
                f"| 理由 | {img.reason} |",
                f"| 图片源 | {img.source_url} |",
                f"| 作者 | {img.author} |",
                f"| 许可 | {img.license} |",
                "",
            ])

    # Candidate only
    for para in paragraphs:
        if not para.chosen_image and para.images:
            report_lines.extend([
                f"### {para.heading_text}（低置信度候选）",
                "",
            ])
            for img in para.images:
                report_lines.extend([
                    f"- `{img.matched_place}` ({img.confidence_score}/100) — {img.reason}",
                    f"  ![候选]({img.image_url})",
                    f"  来源：{img.source_url}",
                    "",
                ])

    # TODO items
    for para in paragraphs:
        if not para.images and not para.chosen_image and para.heading_text:
            query, place = generate_query(para, found_places)
            report_lines.extend([
                f"### ⚠️ {para.heading_text} — 需人工确认",
                "",
                f"未找到置信度 >= {cand_threshold} 的图片。建议搜索：`{query}`",
                "",
            ])

        # Also check for code blocks to skip
        if not para.images and not para.chosen_image and not para.heading_text:
            report_lines.extend([
                f"### 弃用段落（code block / 空内容）",
                "",
                f"跳过配图：段落内容为空或为代码块",
                "",
            ])

    with open(output_report, "w", encoding="utf-8") as f:
        f.write("\n".join(report_lines))

    # Step 8: Insert TODO markers in markdown for low-confidence sections
    # Re-read output, add TODO comments for paragraphs with no chosen image
    with open(output_md, "r", encoding="utf-8") as f:
        md_content = f.read()

    # Add TODO markers after headings that have no auto-inserted image
    md_lines = md_content.split("\n")
    final_lines = []
    for i, line in enumerate(md_lines):
        final_lines.append(line)
        heading_match = re.match(r'^(#{2,6})\s+(.+)$', line.strip())
        if heading_match:
            heading_text = heading_match.group(2).strip()
            for para in paragraphs:
                if para.heading_text == heading_text and not para.chosen_image and para.heading_level >= 1:
                    # Only add TODO if the text below has real content (not just code blocks)
                    non_empty_lines = sum(1 for l in para.paragraph_text.split("\n")
                                          if l.strip() and not l.strip().startswith("```"))
                    if non_empty_lines > 0:
                        final_lines.append(f"<!-- TODO_IMAGE: 需要人工确认：{heading_text} -->")
                    break

    with open(output_md, "w", encoding="utf-8") as f:
        f.write("\n".join(final_lines))

    return {
        "input": input_path,
        "output_md": output_md,
        "output_json": output_json,
        "output_report": output_report,
        "paragraphs_searched": total_searched,
        "auto_inserted": total_auto,
        "downloaded": total_downloaded,
        "candidates": total_candidate,
        "todo_manual": total_todo,
        "found_places": {k: list(v) for k, v in found_places.items()},
    }


# ─── CLI ─────────────────────────────────────────────────────


def main():
    parser = argparse.ArgumentParser(
        description="Go-TeBlog 旅游攻略自动配图预处理脚本",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""示例：
  python3 scripts/travel_image_picker.py 庐山攻略.md
  python3 scripts/travel_image_picker.py 庐山攻略.md --config my-sources.json
""",
    )
    parser.add_argument("input", help="输入的 Markdown 文件路径")
    parser.add_argument("--config", "-c", default="",
                        help="图片来源配置文件路径（可选，默认仅启用 Wikimedia Commons）")
    parser.add_argument("--verbose", "-v", action="store_true", help="详细输出")
    parser.add_argument("--download-dir", "-d", default="/vol1/1000/Docker/Go-Blog/blog-images",
                        help="下载图片到本地目录（如 /vol1/1000/Docker/Go-Blog/blog-images/），并替换输出中的远程 URL 为本地路径")
    args = parser.parse_args()

    if not os.path.exists(args.input):
        print(f"错误：找不到文件 {args.input}", file=sys.stderr)
        sys.exit(1)

    config = load_config(args.config)
    if args.verbose:
        sources_enabled = [k for k, v in config.get("sources", {}).items() if v.get("enabled")]
        print(f"配置: 图片来源 = {sources_enabled}")
        print(f"  自动插入阈值 = {config['confidence_thresholds']['auto_insert']}")
        print(f"  候选阈值 = {config['confidence_thresholds']['candidate_min']}")

    result = process_markdown(args.input, config, args.download_dir)

    print(f"\n处理完成：{os.path.basename(args.input)}")
    print(f"  输出文件（3个）：")
    print(f"    📄 {os.path.basename(result['output_md'])}")
    print(f"    📋 {os.path.basename(result['output_json'])}")
    print(f"    📊 {os.path.basename(result['output_report'])}")
    print(f"  地点识别：{result['found_places']}")
    print(f"  自动插入图片：{result['auto_inserted']} 张")
    if result['downloaded']:
        print(f"  ⬇️  已下载到本地：{result['downloaded']} 张")
    print(f"  低置信度候选：{result['candidates']} 张")
    print(f"  需人工确认段落：{result['todo_manual']} 处")


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        print(f"脚本执行失败：{e}", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        sys.exit(2)
