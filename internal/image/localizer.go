package image

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type LocalizeSummary struct {
	Total     int
	Unique    int
	Localized int
	Failed    int
	Skipped   int
}

type ImageLocalizer struct {
	Client        HTTPClient
	Resolver      Resolver
	DialContext   DialContextFunc
	StorageDir    string
	PublicPrefix  string
	MaxImages     int
	MaxBytes      int64
	MaxConcurrent int
	Timeout       time.Duration
}

type ImageLocalizeResult struct {
	OriginalURL string `json:"original_url"`
	LocalPath   string `json:"local_path,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
}

func (l *ImageLocalizer) withDefaults() ImageLocalizer {
	out := *l
	cfg := GetConfig()
	if out.Resolver == nil {
		out.Resolver = net.DefaultResolver
	}
	if out.DialContext == nil {
		dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		out.DialContext = dialer.DialContext
	}
	if out.StorageDir == "" {
		out.StorageDir = cfg.StorageDir
	}
	if out.PublicPrefix == "" {
		out.PublicPrefix = cfg.PublicPrefix
	}
	if out.MaxImages <= 0 {
		out.MaxImages = cfg.MaxImages
	}
	if out.MaxBytes <= 0 {
		out.MaxBytes = cfg.MaxBytes
	}
	if out.MaxConcurrent <= 0 {
		out.MaxConcurrent = cfg.MaxConcurrent
	}
	if out.Timeout <= 0 {
		out.Timeout = cfg.Timeout
	}
	if out.Client == nil {
		out.Client = NewSSRFHTTPClient(out.Resolver, out.DialContext, out.Timeout)
	}
	return out
}

func (l *ImageLocalizer) Localize(ctx context.Context, content string) (string, LocalizeSummary) {
	if !GetConfig().Enabled {
		return content, LocalizeSummary{}
	}
	cfg := l.withDefaults()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	refs := extractImageRefs(content)
	unique := make(map[string]struct{})
	var urls []string
	for _, ref := range refs {
		if !ref.isRemote || ref.inProtected {
			continue
		}
		if _, ok := unique[ref.url]; ok {
			continue
		}
		unique[ref.url] = struct{}{}
		urls = append(urls, ref.url)
	}

	summary := LocalizeSummary{Total: countRemoteRefs(refs), Unique: len(urls)}
	if len(urls) > cfg.MaxImages {
		summary.Skipped += len(urls) - cfg.MaxImages
		urls = urls[:cfg.MaxImages]
	}
	if len(urls) == 0 {
		return content, summary
	}

	type result struct {
		url  string
		path string
		err  error
	}
	jobs := make(chan string)
	results := make(chan result, len(urls))
	workers := cfg.MaxConcurrent
	if workers > len(urls) {
		workers = len(urls)
	}
	for i := 0; i < workers; i++ {
		go func() {
			for u := range jobs {
				path, err := cfg.downloadAndSave(ctx, u)
				results <- result{url: u, path: path, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, u := range urls {
			select {
			case <-ctx.Done():
				return
			case jobs <- u:
			}
		}
	}()

	localMap := make(map[string]string)
	for i := 0; i < len(urls); i++ {
		select {
		case <-ctx.Done():
			summary.Failed += len(urls) - i
			log.Printf("[ImageLocalize] context canceled: %v", ctx.Err())
			return replaceURLsInContent(content, refs, localMap), summary
		case r := <-results:
			if r.err != nil {
				summary.Failed++
				log.Printf("[ImageLocalize] FAILED %s: %v", r.url, r.err)
				continue
			}
			summary.Localized++
			localMap[r.url] = r.path
			log.Printf("[ImageLocalize] DOWNLOADED %s -> %s", r.url, r.path)
		}
	}

	return replaceURLsInContent(content, refs, localMap), summary
}

func countRemoteRefs(refs []imageRef) int {
	total := 0
	for _, ref := range refs {
		if ref.isRemote && !ref.inProtected {
			total++
		}
	}
	return total
}
