package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestVerifiedLandingConfidentialityAndFixedNavigation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("BASE_URL", "https://app.test")
	const access = "synthetic-private-access"
	const refresh = "synthetic-private-refresh"
	hash := strings.Repeat("abcdef", 11)
	for _, kind := range []string{"signup", "email_change"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["token_hash"] != hash {
					t.Error("verification payload changed")
				}
				fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"user":{"id":%q}}`, access, refresh, sdkTestID)
			}))
			defer server.Close()
			t.Setenv("SUPABASE_URL", server.URL)
			t.Setenv("SUPABASE_ANON_KEY", "test-anon")
			router := gin.New()
			router.GET("/auth/callback", HandleSupabaseAuthCallback)
			req := httptest.NewRequest(http.MethodGet, "/auth/callback?token_hash="+hash+"&type="+kind+"&redirect_to=https%3A%2F%2Fevil.test%2F%3Cscript%3E", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			body := rec.Body.String()
			if rec.Code != http.StatusOK || rec.Header().Get("Location") != "" {
				t.Fatal("callback redirected instead of committing first-party page")
			}
			destination := "/dashboard?verified=true"
			if kind == "email_change" {
				destination = "/dashboard?email_changed=true"
			}
			strip := strings.Index(body, "history.replaceState(null, '', '/auth/callback')")
			navigation := strings.Index(body, "window.location.replace(\""+destination+"\")")
			if strip < 0 || navigation <= strip || !strings.Contains(body, `href="`+destination+`"`) {
				t.Fatal("unsafe landing navigation order or target")
			}
			for _, secret := range []string{access, refresh, hash, "evil.test", "redirect_to", "token_hash"} {
				if strings.Contains(body, secret) {
					t.Fatal("private value or caller redirect in landing page")
				}
			}
			if strings.Contains(body, "<script src=") || strings.Contains(body, "https://") {
				t.Fatal("external resource in landing page")
			}
			csp := rec.Header().Get("Content-Security-Policy")
			match := regexp.MustCompile(`<script nonce="([A-Za-z0-9_-]+)">`).FindStringSubmatch(body)
			if len(match) != 2 || !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "script-src 'nonce-"+match[1]+"'") || !strings.Contains(csp, "base-uri 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
				t.Fatal("missing nonce CSP")
			}
			if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing confidentiality headers")
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 2 {
				t.Fatal("missing auth cookies")
			}
			for _, cookie := range cookies {
				if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
					t.Fatal("auth cookie policy weakened")
				}
			}
		})
	}
}

func TestVerifiedLandingRequiresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, kind := range []string{"signup", "email_change"} {
		for _, tokens := range []struct{ access, refresh string }{{"", ""}, {"synthetic-access", ""}, {"", "synthetic-refresh"}} {
			t.Run(kind+tokens.access+tokens.refresh, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"user":{"id":%q}}`, tokens.access, tokens.refresh, sdkTestID)
				}))
				defer server.Close()
				t.Setenv("SUPABASE_URL", server.URL)
				t.Setenv("SUPABASE_ANON_KEY", "test-anon")
				router := gin.New()
				router.GET("/auth/callback", HandleSupabaseAuthCallback)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/callback?token_hash="+strings.Repeat("a", 64)+"&type="+kind, nil))
				if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" || len(rec.Result().Cookies()) != 0 || strings.Contains(rec.Body.String(), "window.location.replace") {
					t.Fatal("missing session reported success")
				}
			})
		}
	}
}
