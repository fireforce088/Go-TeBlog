package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSetAdminSessionCookieUsesSaferDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://example.com/admin", nil)

	setAdminSessionCookie(ctx, "session-123", 1800)

	cookie := recorder.Result().Cookies()[0]
	if cookie.Name != adminSessionCookieName || cookie.Value != "session-123" {
		t.Fatalf("unexpected cookie: %#v", cookie)
	}
	if !cookie.HttpOnly {
		t.Fatal("expected HttpOnly cookie")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
	if !cookie.Secure {
		t.Fatal("expected Secure cookie on HTTPS requests")
	}
}

func TestSetAdminSessionCookieTrustsForwardedProto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://example.com/admin", nil)
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	setAdminSessionCookie(ctx, "session-123", 1800)

	cookie := recorder.Result().Cookies()[0]
	if !cookie.Secure {
		t.Fatal("expected Secure cookie behind HTTPS reverse proxy")
	}
}

func TestAdminCSRFValidationRequiresMatchingCookieAndToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := func(token string) *gin.Context {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "https://example.com/admin/save", strings.NewReader("_csrf="+token))
		c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c.Request.AddCookie(&http.Cookie{Name: adminCSRFCookieName, Value: "token-123"})
		return c
	}

	if !validateAdminCSRFToken(ctx("token-123")) {
		t.Fatal("expected matching CSRF cookie and form token to validate")
	}

	if validateAdminCSRFToken(ctx("wrong")) {
		t.Fatal("expected mismatched CSRF token to fail")
	}
}

func TestAdminCSRFValidationAcceptsHeaderToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "https://example.com/admin/save", strings.NewReader("_csrf=token-123"))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request.Header.Set("X-CSRF-Token", "token-123")
	ctx.Request.AddCookie(&http.Cookie{Name: adminCSRFCookieName, Value: "token-123"})

	if !validateAdminCSRFToken(ctx) {
		t.Fatal("expected matching CSRF cookie and header token to validate")
	}
}

func TestLoginAttemptLimiterLocksAfterRepeatedFailures(t *testing.T) {
	limiter := newLoginAttemptLimiter(3, time.Minute, 5*time.Minute)
	now := time.Unix(1000, 0)
	key := "127.0.0.1|admin"

	for i := 0; i < 3; i++ {
		allowed, _ := limiter.Allow(key, now.Add(time.Duration(i)*time.Second))
		if !allowed {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		limiter.RecordFailure(key, now.Add(time.Duration(i)*time.Second))
	}

	allowed, retryAfter := limiter.Allow(key, now.Add(4*time.Second))
	if allowed {
		t.Fatal("expected limiter to block after repeated failures")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retryAfter, got %d", retryAfter)
	}

	limiter.RecordSuccess(key)
	allowed, _ = limiter.Allow(key, now.Add(5*time.Second))
	if !allowed {
		t.Fatal("expected successful login to reset limiter")
	}
}

func TestGenerateAdminPasswordUsesEightAlphanumericCharacters(t *testing.T) {
	password, err := generateAdminPassword(8)
	if err != nil {
		t.Fatalf("generateAdminPassword failed: %v", err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9]{8}$`).MatchString(password) {
		t.Fatalf("unexpected generated password: %q", password)
	}
}

func TestBuildPostFilterQueryUsesParameters(t *testing.T) {
	filter := adminPostFilter{Search: "%' OR 1=1 --", Status: "publish"}
	where, args := buildPostFilterWhere(filter, false)

	if strings.Contains(where, filter.Search) {
		t.Fatalf("search text should not be interpolated into SQL: %s", where)
	}
	if len(args) != 2 {
		t.Fatalf("unexpected args: %#v", args)
	}
	seen := map[interface{}]bool{}
	for _, arg := range args {
		seen[arg] = true
	}
	if !seen["%"+filter.Search+"%"] || !seen["publish"] {
		t.Fatalf("unexpected args: %#v", args)
	}
}
