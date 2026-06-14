package image

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	testPNG  = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0, 'I', 'H', 'D', 'R'}
	testJPEG = []byte{0xff, 0xd8, 0xff, 0xdb, 0, 0, 0, 0, 0, 0, 0, 0}
	testGIF  = []byte("GIF89a123456")
	testWebP = []byte("RIFFxxxxWEBPVP8 ")
)

type staticResolver map[string][]string

func (r staticResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if ips, ok := r[host]; ok {
		return ips, nil
	}
	return []string{"93.184.216.34"}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func localizerForHandler(t *testing.T, handler http.Handler) *ImageLocalizer {
	t.Helper()
	dir := t.TempDir()
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			return recorder.Result(), nil
		}),
		Timeout: 2 * time.Second,
	}
	return &ImageLocalizer{
		Client:        client,
		Resolver:      staticResolver{"example.com": {"93.184.216.34"}},
		StorageDir:    dir,
		PublicPrefix:  "/usr/uploads/article-images",
		MaxBytes:      1024,
		MaxImages:     20,
		MaxConcurrent: 3,
		Timeout:       2 * time.Second,
	}
}

func TestImageLocalizeSSRFValidation(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"http ok", "http://example.com/a.png", true},
		{"https ok", "https://example.com/a.png", true},
		{"ftp rejected", "ftp://example.com/a.png", false},
		{"userinfo rejected", "http://user@example.com/a.png", false},
		{"single label rejected", "http://localhost/a.png", false},
		{"missing host rejected", "http:///a.png", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.raw, nil)
			if err != nil && tc.ok {
				t.Fatalf("request parse failed: %v", err)
			}
			if err != nil {
				return
			}
			err = validateRemoteURL(req.URL)
			if tc.ok && err != nil {
				t.Fatalf("unexpected reject: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected rejection")
			}
		})
	}
}

func TestImageLocalizeBlockedIPs(t *testing.T) {
	ips := []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.1.1", "100.64.0.1", "0.1.2.3", "::1", "fc00::1", "fe80::1"}
	for _, raw := range ips {
		t.Run(raw, func(t *testing.T) {
			if !isBlockedIP(net.ParseIP(raw)) {
				t.Fatalf("%s should be blocked", raw)
			}
		})
	}
}

func TestImageLocalizeRedirectValidation(t *testing.T) {
	client := NewSSRFHTTPClient(staticResolver{"example.com": {"93.184.216.34"}}, nil, time.Second)
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/x.png", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(req, nil); err == nil {
		t.Fatal("expected redirect to private IP to fail")
	}
}

func TestImageLocalizeMarkdownAndHTML(t *testing.T) {
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(testPNG); err != nil {
			t.Fatal(err)
		}
	})

	localizer := localizerForHandler(t, handler)
	baseURL := "http://example.com"
	content := fmt.Sprintf("![a](%s/a.png)\n<img alt=\"b\" src=\"%s/b.png\">", baseURL, baseURL)
	got, summary := localizer.Localize(context.Background(), content)
	if summary.Total != 2 || summary.Unique != 2 || summary.Localized != 2 || summary.Failed != 0 {
		t.Fatalf("bad summary: %+v", summary)
	}
	if strings.Contains(got, baseURL) || strings.Count(got, "/usr/uploads/article-images/") != 2 {
		t.Fatalf("content not localized: %s", got)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits, got %d", hits.Load())
	}
}

func TestImageLocalizeDeduplicates(t *testing.T) {
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(testPNG); err != nil {
			t.Fatal(err)
		}
	})
	localizer := localizerForHandler(t, handler)
	baseURL := "http://example.com"
	content := fmt.Sprintf("![a](%s/a.png) <img src=\"%s/a.png\">", baseURL, baseURL)
	got, summary := localizer.Localize(context.Background(), content)
	if summary.Total != 2 || summary.Unique != 1 || summary.Localized != 1 {
		t.Fatalf("bad summary: %+v", summary)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one download, got %d", hits.Load())
	}
	if strings.Contains(got, baseURL) {
		t.Fatalf("duplicate URL was not replaced everywhere: %s", got)
	}
}

func TestImageLocalizeSkipsLocalPaths(t *testing.T) {
	cases := []string{
		"![a](/usr/uploads/a.png)",
		"![b](./b.png)",
		"![c](../c.png)",
		"<img src=\"data:image/png;base64,abc\">",
	}
	for _, content := range cases {
		t.Run(content, func(t *testing.T) {
			refs := extractImageRefs(content)
			for _, ref := range refs {
				if ref.isRemote {
					t.Fatalf("expected local/data ref to be skipped: %+v", ref)
				}
			}
		})
	}
}

func TestImageLocalizeSupportedFormats(t *testing.T) {
	cases := map[string][]byte{"jpg": testJPEG, "png": testPNG, "gif": testGIF, "webp": testWebP}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/"+name)
				if name == "jpg" {
					w.Header().Set("Content-Type", "image/jpeg")
				}
				if _, err := w.Write(body); err != nil {
					t.Fatal(err)
				}
			})
			baseURL := "http://example.com"
			got, summary := localizerForHandler(t, handler).Localize(context.Background(), fmt.Sprintf("![x](%s/x)", baseURL))
			if summary.Localized != 1 || strings.Contains(got, baseURL) {
				t.Fatalf("format not localized: summary=%+v content=%s", summary, got)
			}
		})
	}
}

func TestImageLocalizeRejectsSVG(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		if _, err := w.Write([]byte("<svg></svg>")); err != nil {
			t.Fatal(err)
		}
	})
	content := "![x](http://example.com/x.svg)"
	got, summary := localizerForHandler(t, handler).Localize(context.Background(), content)
	if summary.Failed != 1 || got != content {
		t.Fatalf("expected SVG failure and original content, summary=%+v got=%s", summary, got)
	}
}

func TestImageLocalizeHTTPErrorAndContentType(t *testing.T) {
	cases := []struct {
		name   string
		status int
		ct     string
		body   []byte
	}{
		{"404", http.StatusNotFound, "image/png", testPNG},
		{"500", http.StatusInternalServerError, "image/png", testPNG},
		{"text", http.StatusOK, "text/plain", testPNG},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.ct)
				w.WriteHeader(tc.status)
				if _, err := w.Write(tc.body); err != nil {
					t.Fatal(err)
				}
			})
			_, summary := localizerForHandler(t, handler).Localize(context.Background(), "![x](http://example.com/x)")
			if summary.Failed != 1 {
				t.Fatalf("expected failure: %+v", summary)
			}
		})
	}
}

func TestImageLocalizeTimeoutAndSizeLimit(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.Header().Set("Content-Type", "image/png")
			if _, err := w.Write(testPNG); err != nil {
				return
			}
		})
		localizer := localizerForHandler(t, handler)
		localizer.Timeout = time.Nanosecond
		_, summary := localizer.Localize(context.Background(), "![x](http://example.com/x)")
		if summary.Failed != 1 {
			t.Fatalf("expected timeout failure: %+v", summary)
		}
	})
	t.Run("too large", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			if _, err := w.Write(append(testPNG, make([]byte, 100)...)); err != nil {
				t.Fatal(err)
			}
		})
		localizer := localizerForHandler(t, handler)
		localizer.MaxBytes = 8
		_, summary := localizer.Localize(context.Background(), "![x](http://example.com/x)")
		if summary.Failed != 1 {
			t.Fatalf("expected size failure: %+v", summary)
		}
	})
}

func TestImageLocalizePartialFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "bad") {
			http.Error(w, "bad", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(testPNG); err != nil {
			t.Fatal(err)
		}
	})
	baseURL := "http://example.com"
	content := fmt.Sprintf("![ok](%s/ok.png) ![bad](%s/bad.png)", baseURL, baseURL)
	got, summary := localizerForHandler(t, handler).Localize(context.Background(), content)
	if summary.Localized != 1 || summary.Failed != 1 {
		t.Fatalf("expected partial result: %+v", summary)
	}
	if !strings.Contains(got, baseURL+"/bad.png") || strings.Contains(got, baseURL+"/ok.png") {
		t.Fatalf("unexpected replacement: %s", got)
	}
}

func TestImageLocalizeProtectedMarkdownBlocks(t *testing.T) {
	cases := []string{
		"```mermaid\nflowchart LR\nA[![x](http://example.com/a.png)]\n```\n",
		"````mermaid\n![x](http://example.com/a.png)\n````\n",
		"```go\nvar s = \"![x](http://example.com/a.png)\"\n```\n",
		"    ![x](http://example.com/a.png)\n",
		"`![x](http://example.com/a.png)`",
	}
	for i, content := range cases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			refs := extractImageRefs(content)
			for _, ref := range refs {
				if ref.isRemote && !ref.inProtected {
					t.Fatalf("expected protected ref: %+v", ref)
				}
			}
		})
	}
}

func TestImageLocalizeTablesAndHTMLProtected(t *testing.T) {
	t.Run("table image", func(t *testing.T) {
		refs := extractImageRefs("| a |\n| - |\n| ![x](http://example.com/a.png) |\n")
		if len(refs) != 1 || !refs[0].isRemote || refs[0].inProtected {
			t.Fatalf("expected table image ref: %+v", refs)
		}
	})
	t.Run("table local skipped", func(t *testing.T) {
		refs := extractImageRefs("| a |\n| - |\n| ![x](/blog/usr/a.png) |\n")
		if len(refs) != 1 || refs[0].isRemote {
			t.Fatalf("expected local table ref skipped: %+v", refs)
		}
	})
	for _, tag := range []string{"script", "style", "pre", "code", "textarea"} {
		t.Run(tag, func(t *testing.T) {
			content := fmt.Sprintf("<%s><img src=\"http://example.com/a.png\"></%s>", tag, tag)
			refs := extractImageRefs(content)
			for _, ref := range refs {
				if ref.isRemote && !ref.inProtected {
					t.Fatalf("expected protected HTML ref: %+v", ref)
				}
			}
		})
	}
}

func TestImageLocalizeConcurrencyAndLimit(t *testing.T) {
	t.Run("concurrency bounded", func(t *testing.T) {
		var active atomic.Int32
		var maxActive atomic.Int32
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := active.Add(1)
			for {
				old := maxActive.Load()
				if now <= old || maxActive.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			w.Header().Set("Content-Type", "image/png")
			if _, err := w.Write(testPNG); err != nil {
				t.Fatal(err)
			}
		})
		var parts []string
		baseURL := "http://example.com"
		for i := 0; i < 6; i++ {
			parts = append(parts, fmt.Sprintf("![x](%s/%d.png)", baseURL, i))
		}
		localizer := localizerForHandler(t, handler)
		localizer.MaxConcurrent = 2
		_, summary := localizer.Localize(context.Background(), strings.Join(parts, "\n"))
		if summary.Localized != 6 || maxActive.Load() > 2 {
			t.Fatalf("bad concurrency summary=%+v max=%d", summary, maxActive.Load())
		}
	})
	t.Run("max images", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			if _, err := w.Write(testPNG); err != nil {
				t.Fatal(err)
			}
		})
		var parts []string
		baseURL := "http://example.com"
		for i := 0; i < 5; i++ {
			parts = append(parts, fmt.Sprintf("![x](%s/%d.png)", baseURL, i))
		}
		localizer := localizerForHandler(t, handler)
		localizer.MaxImages = 3
		_, summary := localizer.Localize(context.Background(), strings.Join(parts, "\n"))
		if summary.Localized != 3 || summary.Skipped != 2 {
			t.Fatalf("bad limit summary: %+v", summary)
		}
	})
}

func TestImageLocalizeStorageHashAndReuse(t *testing.T) {
	var hits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(testPNG); err != nil {
			t.Fatal(err)
		}
	})
	localizer := localizerForHandler(t, handler)
	got, summary := localizer.Localize(context.Background(), "![x](http://example.com/a.png)")
	if summary.Localized != 1 {
		t.Fatalf("bad summary: %+v", summary)
	}
	start := strings.Index(got, "/usr/uploads/article-images/")
	if start < 0 {
		t.Fatalf("missing public path: %s", got)
	}
	path := strings.TrimSuffix(strings.TrimPrefix(got[start:], "/usr/uploads/article-images/"), ")")
	base := filepath.Base(path)
	if len(strings.TrimSuffix(base, filepath.Ext(base))) != 64 {
		t.Fatalf("filename does not contain full sha256: %s", base)
	}
	if _, err := os.Stat(filepath.Join(localizer.StorageDir, filepath.FromSlash(path))); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}
}

func TestImageLocalizeAllFailedKeepsContent(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("boom")
	})}
	localizer := &ImageLocalizer{Client: client, StorageDir: t.TempDir(), MaxBytes: 1024, Timeout: time.Second}
	content := "![x](http://example.com/a.png)"
	got, summary := localizer.Localize(context.Background(), content)
	if got != content || summary.Failed != 1 || summary.Localized != 0 {
		t.Fatalf("expected original content and failure: got=%s summary=%+v", got, summary)
	}
}

func TestImageLocalizeNoGoroutineDataRaceSmoke(t *testing.T) {
	var mu sync.Mutex
	seen := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen++
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(testPNG); err != nil {
			t.Fatal(err)
		}
	})
	localizer := localizerForHandler(t, handler)
	baseURL := "http://example.com"
	content := fmt.Sprintf("![a](%s/a.png)\n![b](%s/b.png)", baseURL, baseURL)
	_, summary := localizer.Localize(context.Background(), content)
	if summary.Localized != 2 {
		t.Fatalf("bad summary: %+v", summary)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen != 2 {
		t.Fatalf("expected 2 requests, got %d", seen)
	}
}
