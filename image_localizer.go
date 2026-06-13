package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"log"
)

type ImageLocalizeResult struct {
	OriginalURL string `json:"original_url"`
	LocalPath   string `json:"local_path,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
}

// 环境变量读取
func getImageLocalizeEnabled() bool { return os.Getenv("IMAGE_LOCALIZE_ENABLED") != "false" }
func getImageLocalizeMaxSize() int64 {
	s := os.Getenv("IMAGE_LOCALIZE_MAX_SIZE_MB")
	if s == "" { return 10 }
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
func getImageLocalizeTimeout() int {
	s := os.Getenv("IMAGE_LOCALIZE_TIMEOUT_SEC")
	if s == "" { return 15 }
	n, _ := strconv.Atoi(s)
	return n
}
func getImageLocalizeDir() string {
	s := os.Getenv("IMAGE_LOCALIZE_DIR")
	if s == "" { return "uploads/images" }
	return s
}

// 允许的图片扩展名
var allowedExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// Markdown 图片正则：![alt](url)
var mdImageRe = regexp.MustCompile(`!\[.*?\]\((https?://[^)\s]+)\)`)

// HTML <img> 标签正则
var htmlImgRe = regexp.MustCompile(`<img[^>]+src="([^"]+)"[^>]*>`)

func LocalizeRemoteImages(content string) (string, []ImageLocalizeResult) {
	if !getImageLocalizeEnabled() {
		return content, nil
	}
	var results []ImageLocalizeResult
	seen := make(map[string]bool) // 去重

	// ---- 第一遍：Markdown 图片 ----
	localized := mdImageRe.ReplaceAllStringFunc(content, func(match string) string {
		parts := mdImageRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		url := parts[1]
		if seen[url] {
			for _, r := range results {
				if r.OriginalURL == url && r.LocalPath != "" {
					alt := extractAlt(match)
					return fmt.Sprintf("![%s](%s)", alt, r.LocalPath)
				}
			}
		}
		if !isAllowedURL(url) {
			log.Printf("[ImageLocalize] REJECTED %s", url)
			results = append(results, ImageLocalizeResult{OriginalURL: url, Status: "rejected", Error: "blocked URL"})
			seen[url] = true
			return match
		}
		localPath, size, err := downloadImage(url)
		if err != nil {
			log.Printf("[ImageLocalize] FAILED %s: %v", url, err)
			results = append(results, ImageLocalizeResult{OriginalURL: url, Status: "failed", Error: err.Error()})
			seen[url] = true
			return match
		}
		log.Printf("[ImageLocalize] DOWNLOADED %s -> %s (%d bytes)", url, localPath, size)
		results = append(results, ImageLocalizeResult{OriginalURL: url, LocalPath: localPath, Status: "downloaded", FileSize: size})
		seen[url] = true
		alt := extractAlt(match)
		return fmt.Sprintf("![%s](%s)", alt, localPath)
	})

	// ---- 第二遍：HTML <img> 标签 ----
	localizedFinal := htmlImgRe.ReplaceAllStringFunc(localized, func(match string) string {
		parts := htmlImgRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		url := parts[1]
		if seen[url] {
			for _, r := range results {
				if r.OriginalURL == url && r.LocalPath != "" {
					return strings.Replace(match, url, r.LocalPath, 1)
				}
			}
			return match
		}
		// 已是本地/MinIO 路径，跳过
		if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") ||
			strings.HasPrefix(url, "data:") || strings.Contains(url, "img.w-tx.top") {
			seen[url] = true
			return match
		}
		if !isAllowedURL(url) {
			log.Printf("[ImageLocalize] REJECTED <img> %s", url)
			results = append(results, ImageLocalizeResult{OriginalURL: url, Status: "rejected", Error: "blocked URL"})
			seen[url] = true
			return match
		}
		localPath, size, err := downloadImage(url)
		if err != nil {
			log.Printf("[ImageLocalize] FAILED <img> %s: %v", url, err)
			results = append(results, ImageLocalizeResult{OriginalURL: url, Status: "failed", Error: err.Error()})
			seen[url] = true
			return match
		}
		log.Printf("[ImageLocalize] DOWNLOADED <img> %s -> %s (%d bytes)", url, localPath, size)
		results = append(results, ImageLocalizeResult{OriginalURL: url, LocalPath: localPath, Status: "downloaded", FileSize: size})
		seen[url] = true
		return strings.Replace(match, url, localPath, 1)
	})

	return localizedFinal, results
}
func isAllowedURL(rawURL string) bool {
	// Must be http or https
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return false
	}
	// Check extension
	ext := strings.ToLower(filepath.Ext(rawURL))
	if !allowedExt[ext] && ext != "" {
		return false
	}
	// Parse host
	host := extractHost(rawURL)
	if host == "" {
		return false
	}
	// Resolve IP and check for private/rfc1918
	ips, err := net.LookupHost(host)
	if err != nil {
		// Can't resolve - still allow, download will fail later
		return true
	}
	for _, ip := range ips {
		if isPrivateIP(net.ParseIP(ip)) {
			return false
		}
	}
	return true
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil { return false }
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return true
	}
	return false
}

func extractHost(rawURL string) string {
	// Simple host extraction from URL
	after := strings.TrimPrefix(rawURL, "https://")
	after = strings.TrimPrefix(after, "http://")
	idx := strings.Index(after, "/")
	if idx >= 0 { after = after[:idx] }
	idx = strings.Index(after, ":")
	if idx >= 0 { after = after[:idx] }
	return after
}

func extractAlt(match string) string {
	// Extract alt text from ![alt](url)
	idx1 := strings.Index(match, "[")
	idx2 := strings.Index(match, "]")
	if idx1 >= 0 && idx2 > idx1 {
		return match[idx1+1:idx2]
	}
	return ""
}

func downloadImage(url string) (localPath string, size int64, err error) {
	maxSize := getImageLocalizeMaxSize() * 1024 * 1024
	timeout := getImageLocalizeTimeout()
	baseDir := getImageLocalizeDir()

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Get(url)
	if err != nil { return "", 0, fmt.Errorf("download failed: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Check Content-Type
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "", 0, fmt.Errorf("Content-Type is %s, not image/*", ct)
	}

	// Check Content-Length
	if resp.ContentLength > maxSize {
		return "", 0, fmt.Errorf("Content-Length %d exceeds max %d", resp.ContentLength, maxSize)
	}

	// Read body with limit
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil { return "", 0, fmt.Errorf("read failed: %w", err) }
	if int64(len(data)) > maxSize {
		return "", 0, fmt.Errorf("body %d exceeds max %d", len(data), maxSize)
	}

	// Validate image via magic bytes
	if !isValidImage(data) {
		return "", 0, fmt.Errorf("image validation failed")
	}

	// Determine filename
	sha := fmt.Sprintf("%x", sha1.Sum([]byte(url)))[:12]
	origFilename := extractFilename(url, ct)
	filename := sha + "-" + origFilename

	// Determine directory: uploads/images/YYYY/MM/
	now := time.Now()
	dir := filepath.Join(baseDir, now.Format("2006"), now.Format("01"))
	absDir := filepath.Join(".", dir)

	// Create directory
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return "", 0, fmt.Errorf("mkdir failed: %w", err)
	}

	localPath = "/" + dir + "/" + filename

	// Check if file already exists (dedup)
	absPath := filepath.Join(absDir, filename)
	if _, err := os.Stat(absPath); err == nil {
		return localPath, int64(len(data)), nil // already exists
	}

	// Write file
	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return "", 0, fmt.Errorf("write failed: %w", err)
	}

	return localPath, int64(len(data)), nil
}

func extractFilename(url, contentType string) string {
	// Try to get filename from URL path
	afterSlash := strings.TrimSuffix(url, "/")
	idx := strings.LastIndex(afterSlash, "/")
	if idx >= 0 && idx < len(afterSlash)-1 {
		candidate := afterSlash[idx+1:]
		// If candidate has an extension we allow, use it
		ext := strings.ToLower(filepath.Ext(candidate))
		if allowedExt[ext] {
			return candidate
		}
		// If candidate has some extension but not allowed, still use it
		if ext != "" {
			return candidate
		}
	}
	// Infer from Content-Type
	ext := mimeToExt(contentType)
	return "image" + ext
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg": return ".jpg"
	case "image/png":  return ".png"
	case "image/webp": return ".webp"
	case "image/gif":  return ".gif"
	default:           return ".jpg"
	}
}

func isValidImage(data []byte) bool {
	// Check by magic bytes
	if len(data) < 12 { return false }
	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF { return true }
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 { return true }
	// GIF: 47 49 46 38
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 { return true }
	// WebP: 52 49 46 46 .... 57 45 42 50
	if len(data) > 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 { return true }
	return false
}

// IsImageLocalizeEnabled is the exported check (not a duplicate)
func IsImageLocalizeEnabled() bool {
	return getImageLocalizeEnabled()
}

// IsLocalPath checks if a URL is already a local path
func IsLocalPath(rawURL string) bool {
	return strings.HasPrefix(rawURL, "/") ||
		strings.HasPrefix(rawURL, "./") ||
		strings.HasPrefix(rawURL, "../") ||
		strings.HasPrefix(rawURL, "data:image/")
}
