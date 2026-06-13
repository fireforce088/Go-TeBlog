package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ─── 配置 ──────────────────────────────────────────────

var (
	listenAddr  = env("IMG_FIXER_LISTEN", ":8900")
	minioAlias  = env("MINIO_ALIAS", "local")
	minioBucket = env("MINIO_BUCKET", "blog-images")
	minioPubURL = env("MINIO_PUBLIC_URL", "https://img.w-tx.top")
	workDir     = env("IMG_FIXER_WORK_DIR", "/tmp/img-fixer")
	userAgent   = "Go-TeBlog-ImageFixer/1.0 (https://blog.w-tx.top)"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ─── 搜索请求 ──────────────────────────────────────────

type SearchReq struct {
	Query    string `json:"query"`
	Filename string `json:"filename,omitempty"`
}

type SearchResp struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	Title  string `json:"title"`
	Error  string `json:"error,omitempty"`
}

// ─── Commons API ───────────────────────────────────────

type CommonsFile struct {
	Title     string `json:"title"`
	ImageURL  string `json:"url"`
	ThumbURL  string `json:"thumburl"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	MIME      string `json:"mime"`
}

func searchCommons(query string, limit int) ([]CommonsFile, error) {
	apiURL := fmt.Sprintf(
		"https://commons.wikimedia.org/w/api.php?action=query&list=search&srsearch=%s&srnamespace=6&srlimit=%d&format=json",
		url.QueryEscape(query), limit,
	)
	
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", userAgent)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("commons search failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return nil, fmt.Errorf("commons HTTP %d: %s", resp.StatusCode, string(body))
	}
	
	var result struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("commons parse failed: %w", err)
	}
	
	var files []CommonsFile
	for _, item := range result.Query.Search {
		if !strings.HasPrefix(item.Title, "File:") {
			continue
		}
		title := item.Title[5:] // Remove "File:" prefix
		ext := strings.ToLower(filepath.Ext(title))
		if ext == ".pdf" || ext == ".djvu" || ext == ".svg" || ext == ".xml" {
			continue
		}
		
		// Get image info
		info, err := getImageInfo(title)
		if err != nil {
			log.Printf("  INFO FAIL: %s - %v", title, err)
			continue
		}
		if info != nil && info.Width > 500 {
			files = append(files, *info)
		}
		if len(files) >= 3 {
			break
		}
	}
	
	return files, nil
}

func getImageInfo(title string) (*CommonsFile, error) {
	apiURL := fmt.Sprintf(
		"https://commons.wikimedia.org/w/api.php?action=query&titles=File:%s&prop=imageinfo&iiprop=url|size|mime&format=json",
		url.QueryEscape(title),
	)
	
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", userAgent)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var result struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					URL     string `json:"url"`
					ThumbURL string `json:"thumburl"`
					Width   int    `json:"width"`
					Height  int    `json:"height"`
					MIME    string `json:"mime"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	
	for _, page := range result.Query.Pages {
		if len(page.ImageInfo) > 0 {
			ii := page.ImageInfo[0]
			return &CommonsFile{
				Title:    title,
				ImageURL: ii.URL,
				Width:    ii.Width,
				Height:   ii.Height,
				MIME:     ii.MIME,
			}, nil
		}
	}
	return nil, nil
}

// ─── MinIO 上传 ─────────────────────────────────────────

func uploadToMinIO(localPath, filename string) (string, error) {
	cmd := exec.Command("mc", "cp", localPath, fmt.Sprintf("%s/%s/%s", minioAlias, minioBucket, filename))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mc cp failed: %s %v", string(output), err)
	}
	return fmt.Sprintf("%s/%s/%s", minioPubURL, minioBucket, filename), nil
}

// ─── HTTP Handlers ─────────────────────────────────────

func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}
	
	var req SearchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SearchResp{Error: fmt.Sprintf("bad request: %v", err)})
		return
	}
	
	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, SearchResp{Error: "query is required"})
		return
	}
	
	log.Printf("[Search] query=%q", req.Query)
	
	// Search Commons
	files, err := searchCommons(req.Query, 5)
	if err != nil {
		log.Printf("[Search] ERROR: %v", err)
		writeJSON(w, http.StatusInternalServerError, SearchResp{Error: err.Error()})
		return
	}
	
	if len(files) == 0 {
		log.Printf("[Search] NOT FOUND: %q", req.Query)
		writeJSON(w, http.StatusOK, SearchResp{URL: "", Source: "commons", Title: ""})
		return
	}
	
	best := files[0]
	log.Printf("[Search] BEST: %s (%dx%d, %s)", best.Title, best.Width, best.Height, best.ImageURL)
	
	// Download
	os.MkdirAll(workDir, 0755)
	
	ext := filepath.Ext(best.ImageURL)
	if ext == "" {
		ext = ".jpg"
	}
	
	hash := sha256.Sum256([]byte(best.ImageURL))
	filename := fmt.Sprintf("%x%s", hash[:8], ext)
	localPath := filepath.Join(workDir, filename)
	
	if err := downloadFile(best.ImageURL, localPath); err != nil {
		log.Printf("[Search] DOWNLOAD FAIL: %v", err)
		writeJSON(w, http.StatusInternalServerError, SearchResp{Error: fmt.Sprintf("download failed: %v", err)})
		return
	}
	log.Printf("[Search] Downloaded: %s -> %s", best.ImageURL, localPath)
	
	// Upload to MinIO
	pubURL, err := uploadToMinIO(localPath, filename)
	if err != nil {
		log.Printf("[Search] MINIO FAIL: %v", err)
		writeJSON(w, http.StatusInternalServerError, SearchResp{Error: fmt.Sprintf("minio upload failed: %v", err)})
		return
	}
	
	log.Printf("[Search] SUCCESS: %s -> %s", best.Title, pubURL)
	writeJSON(w, http.StatusOK, SearchResp{
		URL:    pubURL,
		Source: "commons",
		Title:  best.Title,
	})
	
	// Cleanup
	go os.Remove(localPath)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─── 辅助 ──────────────────────────────────────────────

func downloadFile(urlStr, localPath string) error {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", userAgent)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("get failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	// Check content type
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		// Try to parse anyway, but warn
		log.Printf("[Download] WARN: Content-Type is %s for %s", ct, urlStr)
	}
	
	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create failed: %w", err)
	}
	defer out.Close()
	
	written, err := io.Copy(out, io.LimitReader(resp.Body, 50<<20)) // 50MB max
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	if written == 0 {
		return fmt.Errorf("empty file")
	}
	
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func init() {
	// Suppress log timestamp for cleaner output
	log.SetFlags(log.LstdFlags)
}

func main() {
	log.Printf("Starting Go-TeBlog Image Fixer Worker on %s", listenAddr)
	log.Printf("MinIO: alias=%s bucket=%s pubURL=%s", minioAlias, minioBucket, minioPubURL)
	log.Printf("Work dir: %s", workDir)
	
	// Ensure MinIO alias is configured
	checkMinIO()
	
	http.HandleFunc("/search", handleSearch)
	http.HandleFunc("/health", handleHealth)
	
	server := &http.Server{
		Addr:         listenAddr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
	}
	
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func checkMinIO() {
	cmd := exec.Command("mc", "ls", fmt.Sprintf("%s/%s", minioAlias, minioBucket))
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("WARN: Cannot access MinIO: %s %v", string(output), err)
		log.Printf("Make sure to run: mc alias set %s http://localhost:9000 USER PASS", minioAlias)
	} else {
		log.Printf("MinIO access OK")
	}
}
