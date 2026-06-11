package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const adminSessionCookieName = "te_auth"
const adminCSRFCookieName = "te_csrf"

type loginAttemptLimiter struct {
	mu       sync.Mutex
	maxFails int
	window   time.Duration
	lockFor  time.Duration
	entries  map[string]loginAttemptEntry
}

type loginAttemptEntry struct {
	failures    []time.Time
	lockedUntil time.Time
}

func newLoginAttemptLimiter(maxFails int, window, lockFor time.Duration) *loginAttemptLimiter {
	if maxFails < 1 {
		maxFails = 5
	}
	if window <= 0 {
		window = 10 * time.Minute
	}
	if lockFor <= 0 {
		lockFor = 10 * time.Minute
	}
	return &loginAttemptLimiter{
		maxFails: maxFails,
		window:   window,
		lockFor:  lockFor,
		entries:  make(map[string]loginAttemptEntry),
	}
}

func (l *loginAttemptLimiter) Allow(key string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if now.Before(entry.lockedUntil) {
		return false, int(entry.lockedUntil.Sub(now).Seconds())
	}
	entry.failures = recentFailures(entry.failures, now.Add(-l.window))
	entry.lockedUntil = time.Time{}
	l.entries[key] = entry
	return true, 0
}

func (l *loginAttemptLimiter) RecordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	entry.failures = append(recentFailures(entry.failures, now.Add(-l.window)), now)
	if len(entry.failures) >= l.maxFails {
		entry.lockedUntil = now.Add(l.lockFor)
		entry.failures = nil
	}
	l.entries[key] = entry
}

func (l *loginAttemptLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func recentFailures(values []time.Time, cutoff time.Time) []time.Time {
	kept := values[:0]
	for _, value := range values {
		if value.After(cutoff) || value.Equal(cutoff) {
			kept = append(kept, value)
		}
	}
	return kept
}

func loginAttemptKey(ip, username string) string {
	return strings.TrimSpace(ip) + "|" + strings.ToLower(strings.TrimSpace(username))
}

func setAdminSessionCookie(c *gin.Context, value string, maxAge int) {
	cookie := &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(c.Writer, cookie)
}

func clearAdminSessionCookie(c *gin.Context) {
	setAdminSessionCookie(c, "", -1)
}

func isSecureRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.TLS != nil {
		return true
	}
	forwardedProto := strings.ToLower(strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]))
	return forwardedProto == "https" || strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on")
}

func newAdminCSRFToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func setAdminCSRFCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminCSRFCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((12 * time.Hour).Seconds()),
		HttpOnly: false,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func ensureAdminCSRFToken(c *gin.Context) string {
	token, err := c.Cookie(adminCSRFCookieName)
	if err != nil || strings.TrimSpace(token) == "" {
		token = newAdminCSRFToken()
	}
	setAdminCSRFCookie(c, token)
	return token
}

func requestCSRFToken(c *gin.Context) string {
	token := strings.TrimSpace(c.GetHeader("X-CSRF-Token"))
	if token != "" {
		return token
	}
	return strings.TrimSpace(c.PostForm("_csrf"))
}

func validateAdminCSRFToken(c *gin.Context) bool {
	cookieToken, err := c.Cookie(adminCSRFCookieName)
	if err != nil {
		return false
	}
	submittedToken := requestCSRFToken(c)
	if strings.TrimSpace(cookieToken) == "" || submittedToken == "" {
		return false
	}
	if len(cookieToken) != len(submittedToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(submittedToken)) == 1
}

func adminCSRFMiddleware(adminPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureAdminCSRFToken(c)
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if validateAdminCSRFToken(c) {
			c.Next()
			return
		}
		if c.GetHeader("X-Requested-With") == "XMLHttpRequest" || strings.Contains(c.GetHeader("Accept"), "application/json") {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "CSRF token invalid"})
		} else {
			c.HTML(http.StatusForbidden, "admin_error.html", gin.H{
				"AdminPath":    adminPath,
				"ErrorTitle":   "请求已拒绝",
				"ErrorMessage": "安全令牌无效或已过期，请刷新页面后重试。",
			})
		}
		c.Abort()
	}
}

type adminPostFilter struct {
	Search string
	Status string
}

type adminCommentFilter struct {
	Search string
	Status string
}

func sanitizePostStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "publish", "draft", "hidden", "private", "waiting":
		return strings.TrimSpace(status)
	default:
		return ""
	}
}

func sanitizeCommentStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "approved", "waiting", "spam":
		return strings.TrimSpace(status)
	default:
		return ""
	}
}

func buildPostFilterWhere(filter adminPostFilter, visitorOnly bool) (string, []interface{}) {
	clauses := []string{"type='post'"}
	args := []interface{}{}
	if visitorOnly {
		clauses = append(clauses, "status='publish'")
	} else if status := sanitizePostStatus(filter.Status); status != "" {
		clauses = append(clauses, "status=?")
		args = append(args, status)
	}
	if filter.Search = strings.TrimSpace(filter.Search); filter.Search != "" {
		clauses = append(clauses, "title LIKE ?")
		args = append(args, "%"+filter.Search+"%")
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func buildCommentFilterWhere(filter adminCommentFilter) (string, []interface{}) {
	clauses := []string{"1=1"}
	args := []interface{}{}
	if status := sanitizeCommentStatus(filter.Status); status != "" {
		clauses = append(clauses, "c.status=?")
		args = append(args, status)
	}
	if filter.Search = strings.TrimSpace(filter.Search); filter.Search != "" {
		like := "%" + filter.Search + "%"
		clauses = append(clauses, "(c.author LIKE ? OR c.text LIKE ? OR p.title LIKE ?)")
		args = append(args, like, like, like)
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func adminListQuerySuffix(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(strings.TrimSpace(params[key])))
	}
	if len(parts) == 0 {
		return ""
	}
	return "&" + strings.Join(parts, "&")
}

func systemMemorySummary(formatMem func(float64) string) (int, string) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			var totalKB, availableKB int64
			for _, line := range strings.Split(string(data), "\n") {
				parts := strings.Fields(line)
				if len(parts) < 2 {
					continue
				}
				value, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					continue
				}
				switch parts[0] {
				case "MemTotal:":
					totalKB = value
				case "MemAvailable:":
					availableKB = value
				}
			}
			if totalKB > 0 {
				totalGB := int((totalKB*1024 + (512 * 1024 * 1024)) / (1024 * 1024 * 1024))
				if availableKB > 0 {
					return totalGB, formatMem(float64(availableKB) / 1024)
				}
				return totalGB, "未知"
			}
		}
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return 0, formatMem(float64(m.Sys-m.Alloc) / (1024 * 1024))
}

func diskFreeSummary() string {
	if runtime.GOOS != "linux" {
		return fmt.Sprintf("不支持 (%s)", runtime.GOOS)
	}
	output, err := exec.Command("df", "-BG", "/").Output()
	if err != nil {
		return "获取失败"
	}
	fields := strings.Fields(string(output))
	if len(fields) < 11 {
		return "解析失败"
	}
	total := strings.TrimSuffix(fields[8], "G")
	free := strings.TrimSuffix(fields[10], "G")
	return free + ".0 GB 可用 / " + total + ".0 GB 总计"
}

func systemLoadSummary() string {
	if runtime.GOOS == "linux" {
		return "读取失败"
	}
	return fmt.Sprintf("不支持 (%s)", runtime.GOOS)
}
