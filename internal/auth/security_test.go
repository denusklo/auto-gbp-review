package auth

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	supa "github.com/supabase-community/supabase-go"
)

func TestCookiePolicyConfiguredScheme(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, base, forwarded string
		secure                bool
	}{
		{"proxy HTTPS", "https://app.test", "http", true},
		{"local HTTP", "http://127.0.0.1:3000", "https", false},
		{"invalid config", "https://app.test/path", "http", true},
		{"missing config", "", "http", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASE_URL", tc.base)
			router := gin.New()
			router.GET("/cookie", func(c *gin.Context) {
				for _, name := range []string{"sb_access_token", "sb_refresh_token", "auth_token", "reset_access_token", "reset_csrf", "recovery_bridge_csrf"} {
					authCookie(c, name, "test-value", 600)
					authCookie(c, name, "", -1)
				}
			})
			req := httptest.NewRequest("GET", "http://internal/cookie", nil)
			req.Header.Set("X-Forwarded-Proto", tc.forwarded)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if req.TLS != nil {
				t.Fatal("test must model cleartext proxy backend")
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 12 {
				t.Fatal("missing cookies")
			}
			for _, cookie := range cookies {
				wantSameSite := http.SameSiteStrictMode
				if cookie.Name != "recovery_bridge_csrf" {
					wantSameSite = http.SameSiteLaxMode
				}
				if cookie.Secure != tc.secure || !cookie.HttpOnly || cookie.SameSite != wantSameSite || cookie.Path != "/" {
					t.Fatal("incorrect cookie policy")
				}
			}
		})
	}
}

func TestLoginLogoutOriginAndProxyCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("BASE_URL", "https://app.test")
	access, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_role": "merchant"}).SignedString([]byte("test-only"))
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/auth/v1/logout" {
			w.WriteHeader(204)
			return
		}
		fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"test-refresh","user":{"id":%q}}`, access, sdkTestID)
	}))
	defer server.Close()
	old := supabaseClient
	defer func() { supabaseClient = old }()
	var err error
	supabaseClient, err = supa.NewClient(server.URL, "test-anon", nil)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/login", SupabaseLogin)
	router.POST("/logout", SupabaseLogout)
	for _, path := range []string{"/login", "/logout"} {
		for _, origin := range []string{"https://evil.test", "", "null", "https://app.test"} {
			t.Run(path+origin, func(t *testing.T) {
				before := calls
				form := url.Values{"email": {"test@example.com"}, "password": {"test-password"}}
				req := httptest.NewRequest("POST", "http://internal"+path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Origin", origin)
				req.Header.Set("X-Forwarded-Proto", "http")
				req.AddCookie(&http.Cookie{Name: "sb_access_token", Value: access})
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if origin != "https://app.test" {
					if rec.Code != 403 || calls != before || len(rec.Result().Cookies()) != 0 {
						t.Fatal("cross-site request changed authentication")
					}
					return
				}
				if rec.Code != 302 || calls != before+1 {
					t.Fatal("same-origin browser submission failed")
				}
				cookies := rec.Result().Cookies()
				want := 2
				if path == "/logout" {
					want = 3
				}
				if len(cookies) != want {
					t.Fatal("incorrect auth cookies")
				}
				for _, cookie := range cookies {
					if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
						t.Fatal("proxy auth cookie insecure")
					}
					if path == "/logout" && cookie.MaxAge >= 0 {
						t.Fatal("logout did not clear cookie")
					}
				}
			})
		}
	}
}

func TestRequestLoggerNeverCapturesCallbackCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := gin.DefaultWriter
	defer func() { gin.DefaultWriter = old }()
	var output bytes.Buffer
	gin.DefaultWriter = &output
	router := gin.New()
	router.Use(SafeRequestLogger(), gin.RecoveryWithWriter(io.Discard))
	router.GET("/auth/callback", func(c *gin.Context) {
		if c.Query("panic") == "yes" {
			panic("synthetic-secret")
		}
		c.String(400, "Rejected")
	})
	for _, suffix := range []string{"", "&panic=yes"} {
		req := httptest.NewRequest("GET", "/auth/callback?token_hash=synthetic-secret&type=recovery&access_token=synthetic-access"+suffix, nil)
		req.Header.Set("Cookie", "sb_access_token=synthetic-cookie")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}
	logs := output.String()
	for _, secret := range []string{"synthetic-secret", "synthetic-access", "synthetic-cookie", "token_hash", "access_token", "type=recovery"} {
		if strings.Contains(logs, secret) {
			t.Fatal("callback credential appeared in logs")
		}
	}
	if strings.Count(logs, "/auth/callback") != 2 {
		t.Fatal("safe logger missing request records")
	}
}
