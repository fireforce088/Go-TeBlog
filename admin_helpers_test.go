package main

import (
	"net/http"
	"net/http/httptest"
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
