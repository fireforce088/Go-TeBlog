package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// ─── 配置 ──────────────────────────────────────────────

type ImageSearcherConfig struct {
	Enabled    bool
	WorkerURL  string // HK-CN2 worker endpoint, e.g. "http://100.119.183.123:8900"
	TimeoutSec int
	MaxPerRun  int // 单次最多处理多少张图
}

func getSearcherConfig() ImageSearcherConfig {
	cfg := GetConfig()
	return ImageSearcherConfig{
		Enabled:    cfg.SearchEnabled,
		WorkerURL:  cfg.SearchWorkerURL,
		TimeoutSec: 120,
		MaxPerRun:  cfg.MaxImages,
	}
}

// 需要搜索替换的图片域名
var searchDomains = []string{
	"upload.wikimedia.org",
	"commons.wikimedia.org",
}

// ─── 结果 ──────────────────────────────────────────────

type SearchFixResult struct {
	OriginalURL string `json:"original_url"`
	NewURL      string `json:"new_url,omitempty"`
	Status      string `json:"status"` // replaced / skipped / not_found / error
	Source      string `json:"source,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ─── Worker API 请求/响应 ──────────────────────────────

type SearchRequest struct {
	Query    string `json:"query"`
	Filename string `json:"filename,omitempty"`
}

type SearchResponse struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Title  string `json:"title"`
	Error  string `json:"error,omitempty"`
}

type FixContentRequest struct {
	Content       string   `json:"content"`
	SearchDomains []string `json:"search_domains"`
	AutoReplace   bool     `json:"auto_replace"`
}

type FixContentResponse struct {
	Content string            `json:"content"`
	Results []SearchFixResult `json:"results"`
}

// ─── 主流程 ──────────────────────────────────────────────

// FixArticleImages 修复文章正文中的失效图片
// 扫描 HTML <img> 和 Markdown 图片标签
// 对失效的（通过 Worker）搜索 Commons 替换
// 返回更新后的正文和结果列表
func FixArticleImages(content string) (string, []SearchFixResult) {
	cfg := getSearcherConfig()
	if !cfg.Enabled || cfg.WorkerURL == "" {
		return content, nil
	}

	var results []SearchFixResult
	seen := make(map[string]bool)

	var refs []imageRef
	for _, ref := range extractImageRefs(content) {
		if ref.inProtected || seen[ref.url] {
			continue
		}
		seen[ref.url] = true
		if needsSearch(ref.url) {
			refs = append(refs, ref)
		}
	}

	if len(refs) == 0 {
		return content, nil
	}
	log.Printf("[ImageSearcher] Found %d images needing search/replace", len(refs))

	if cfg.MaxPerRun > 0 && len(refs) > cfg.MaxPerRun {
		refs = refs[:cfg.MaxPerRun]
	}

	// 2. 对每张图：通过 Worker 搜索 + 上传到 MinIO
	for _, ref := range refs {
		query := extractQueryFromAlt(content, ref.url)
		log.Printf("[ImageSearcher] Searching: %s (query=%q)", ref.url, query)

		newURL, err := callSearchWorker(cfg.WorkerURL, query, "")
		if err != nil {
			log.Printf("[ImageSearcher] FAILED %s: %v", ref.url, err)
			results = append(results, SearchFixResult{
				OriginalURL: ref.url,
				Status:      "error",
				Error:       err.Error(),
			})
			continue
		}
		if newURL == "" {
			log.Printf("[ImageSearcher] NOT FOUND: %s", ref.url)
			results = append(results, SearchFixResult{
				OriginalURL: ref.url,
				Status:      "not_found",
			})
			continue
		}

		content = strings.Replace(content, ref.url, newURL, 1)

		log.Printf("[ImageSearcher] REPLACED: %s -> %s", ref.url, newURL)
		results = append(results, SearchFixResult{
			OriginalURL: ref.url,
			NewURL:      newURL,
			Status:      "replaced",
			Source:      "commons",
		})
	}

	return content, results
}

// ─── 辅助 ──────────────────────────────────────────────

// needsSearch 判断 URL 是否需要搜索替换
func needsSearch(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	// 已本地化或 MinIO 的 URL 不需要
	if strings.HasPrefix(rawURL, "/") ||
		strings.HasPrefix(rawURL, "./") ||
		strings.HasPrefix(rawURL, "../") ||
		strings.HasPrefix(rawURL, "data:") ||
		strings.Contains(u.Host, "img.w-tx.top") {
		return false
	}
	// 针对特定域名
	for _, d := range searchDomains {
		if strings.Contains(u.Host, d) {
			return true
		}
	}
	return false
}

// extractQueryFromAlt 从图片周围的文本中提取搜索关键词
func extractQueryFromAlt(content, imgURL string) string {
	// 优先从 <em> 图注中提取中文描述
	emRe := htmlEmRe
	emMatches := emRe.FindStringSubmatch(content)
	if len(emMatches) > 1 {
		txt := strings.TrimSpace(emMatches[1])
		// 去掉 "｜ CC BY-SA" 等后缀
		if idx := strings.Index(txt, "｜"); idx > 0 {
			txt = strings.TrimSpace(txt[:idx])
		}
		if len(txt) > 3 {
			return txt
		}
	}

	// 从文件名中推断
	filename := filepath.Base(imgURL)
	filename = strings.TrimSuffix(filename, filepath.Ext(filename))
	filename = strings.ReplaceAll(filename, "_", " ")
	filename = strings.ReplaceAll(filename, "-", " ")
	// 去掉尺寸前缀如 "800px-"
	filename = dimensionPrefixRe.ReplaceAllString(filename, "")
	return strings.TrimSpace(filename)
}

// callSearchWorker 调用 HK-CN2 Worker 搜索图片并上传到 MinIO
// 返回可直接引用的 MinIO URL
func callSearchWorker(workerURL, query, filename string) (string, error) {
	req := SearchRequest{
		Query:    query,
		Filename: filename,
	}
	body, _ := json.Marshal(req)

	client := &http.Client{Timeout: 60 * time.Second}
	apiURL := strings.TrimRight(workerURL, "/") + "/search"

	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("worker call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB

	if resp.StatusCode != http.StatusOK {
		maxLen := 200
		if len(respBody) < maxLen {
			maxLen = len(respBody)
		}
		return "", fmt.Errorf("worker HTTP %d: %s", resp.StatusCode, string(respBody[:maxLen]))
	}

	var sr SearchResponse
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return "", fmt.Errorf("worker response parse: %w", err)
	}
	if sr.Error != "" {
		return "", fmt.Errorf("worker error: %s", sr.Error)
	}
	if sr.URL == "" {
		return "", nil // not found
	}
	return sr.URL, nil
}

// callFixContentWorker 批量处理整篇正文（备用）
func callFixContentWorker(workerURL, content string) (string, []SearchFixResult, error) {
	req := FixContentRequest{
		Content:       content,
		SearchDomains: searchDomains,
		AutoReplace:   true,
	}
	body, _ := json.Marshal(req)

	client := &http.Client{Timeout: 300 * time.Second}
	apiURL := strings.TrimRight(workerURL, "/") + "/fix-content"

	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return content, nil, fmt.Errorf("worker fix-content call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB

	if resp.StatusCode != http.StatusOK {
		maxLen := 200
		if len(respBody) < maxLen {
			maxLen = len(respBody)
		}
		return content, nil, fmt.Errorf("worker HTTP %d: %s", resp.StatusCode, string(respBody[:maxLen]))
	}

	var fr FixContentResponse
	if err := json.Unmarshal(respBody, &fr); err != nil {
		return content, nil, fmt.Errorf("worker response parse: %w", err)
	}
	return fr.Content, fr.Results, nil
}

// ImageSearcherEnabled 检查搜索功能是否启用
func ImageSearcherEnabled() bool {
	cfg := getSearcherConfig()
	return cfg.Enabled && cfg.WorkerURL != ""
}
