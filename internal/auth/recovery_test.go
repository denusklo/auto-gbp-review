package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	supa "github.com/supabase-community/supabase-go"
)

func recoveryTestToken(t *testing.T, method string, age time.Duration) string {
	t.Helper()
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sdkTestID, "session_id": "test-session-id", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"amr": []map[string]interface{}{{"method": method, "timestamp": now.Add(-age).Unix()}},
	}).SignedString([]byte("test-only"))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestRecoveryFragmentBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/auth/callback", HandleSupabaseAuthCallback)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/callback", nil))
	body := rec.Body.String()
	strip := strings.Index(body, "history.replaceState")
	post := strings.Index(body, "fetch('/auth/recovery-session'")
	if rec.Code != 200 || strip < 0 || post < strip || strings.Contains(body, "https://") || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("unsafe or missing fragment bridge")
	}
	if !strings.Contains(body, "csrf:\"") {
		t.Fatal("nonce not safely JSON encoded")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("bridge cacheable")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("bridge cookie insecure")
	}
}

func TestRecoverySessionValidationAndClearing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("BASE_URL", "http://app.test")
	good := recoveryTestToken(t, "recovery", 0)
	otp := recoveryTestToken(t, "otp", 0)
	forged := recoveryTestToken(t, "otp", 2*time.Second)
	ordinary := recoveryTestToken(t, "password", 0)
	stale := recoveryTestToken(t, "otp", 11*time.Minute)
	mutate := func(change func(jwt.MapClaims)) string {
		claims := jwt.MapClaims{}
		_, _, _ = jwt.NewParser().ParseUnverified(otp, claims)
		change(claims)
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-only"))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	wrongSubject := mutate(func(c jwt.MapClaims) { c["sub"] = "another-user" })
	missingSID := mutate(func(c jwt.MapClaims) { delete(c, "session_id") })
	missingAMR := mutate(func(c jwt.MapClaims) { delete(c, "amr") })
	oldIssued := mutate(func(c jwt.MapClaims) { c["iat"] = time.Now().Add(-11 * time.Minute).Unix() })
	missingIssued := mutate(func(c jwt.MapClaims) { delete(c, "iat") })
	expired := mutate(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Second).Unix() })
	futureIssued := mutate(func(c jwt.MapClaims) { c["iat"] = time.Now().Add(time.Minute).Unix() })
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer invalid" || r.Header.Get("Authorization") == "Bearer "+forged {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"msg":"invalid"}`)
			return
		}
		if r.Method == "PUT" {
			putCount++
		}
		fmt.Fprintf(w, `{"id":%q,"email":"test@example.com"}`, sdkTestID)
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
	router.POST("/auth/recovery-session", RecoverySession)
	router.POST("/auth/reset-password", ResetPasswordCallback)
	csrf := strings.Repeat("a", 43)
	for _, tc := range []struct {
		name, token, origin, nonce string
		status                     int
	}{
		{"valid recovery", good, "http://app.test", csrf, 204},
		{"valid OTP", otp, "http://app.test", csrf, 204},
		{"forged OTP rejected online", forged, "http://app.test", csrf, 401},
		{"invalid access", "invalid", "http://app.test", csrf, 401},
		{"wrong subject", wrongSubject, "http://app.test", csrf, 401},
		{"missing session", missingSID, "http://app.test", csrf, 401},
		{"missing AMR", missingAMR, "http://app.test", csrf, 401},
		{"old issued token", oldIssued, "http://app.test", csrf, 401},
		{"missing issued token", missingIssued, "http://app.test", csrf, 401},
		{"expired token", expired, "http://app.test", csrf, 401},
		{"future issued token", futureIssued, "http://app.test", csrf, 401},
		{"ordinary session", ordinary, "http://app.test", csrf, 401},
		{"old recovery", stale, "http://app.test", csrf, 401},
		{"foreign origin", good, "http://evil.test", csrf, 403},
		{"missing origin", good, "", csrf, 403},
		{"wrong nonce", good, "http://app.test", "wrong", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{"access_token": tc.token, "refresh_token": "test-refresh", "csrf": tc.nonce})
			req := httptest.NewRequest("POST", "/auth/recovery-session", strings.NewReader(string(payload)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(&http.Cookie{Name: "recovery_bridge_csrf", Value: csrf})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d want %d", rec.Code, tc.status)
			}
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == "reset_access_token" && (tc.status != 204 || cookie.MaxAge <= 0 || cookie.MaxAge > 600 || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode) {
					t.Fatal("bad recovery cookie")
				}
			}
			if tc.status != 204 {
				return
			}
			cookies := rec.Result().Cookies()
			beforeUpdates := putCount
			form := url.Values{"password": {"new-password"}, "confirm_password": {"new-password"}}
			for _, cookie := range cookies {
				if cookie.Name == "reset_csrf" {
					form.Set("csrf", cookie.Value)
				}
			}
			// Lax recovery cookies must never authorize a cross-origin password
			// POST, even when the attacker supplies a matching CSRF form value.
			for _, origin := range []string{"http://evil.test", ""} {
				crossReq := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(form.Encode()))
				crossReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				crossReq.Header.Set("Origin", origin)
				for _, cookie := range cookies {
					if cookie.MaxAge > 0 {
						crossReq.AddCookie(cookie)
					}
				}
				crossRec := httptest.NewRecorder()
				router.ServeHTTP(crossRec, crossReq)
				if crossRec.Code != http.StatusForbidden || putCount != beforeUpdates {
					t.Fatal("cross-origin password POST accepted")
				}
			}
			resetReq := httptest.NewRequest("POST", "/auth/reset-password", strings.NewReader(form.Encode()))
			resetReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resetReq.Header.Set("Origin", "http://app.test")
			for _, cookie := range cookies {
				if cookie.MaxAge > 0 {
					resetReq.AddCookie(cookie)
				}
			}
			resetRec := httptest.NewRecorder()
			router.ServeHTTP(resetRec, resetReq)
			if resetRec.Code != 302 || resetRec.Header().Get("Location") != "/login?password_reset=true" || putCount != beforeUpdates+1 {
				t.Fatal("password reset failed")
			}
			cleared := 0
			for _, cookie := range resetRec.Result().Cookies() {
				if cookie.MaxAge < 0 {
					cleared++
				}
			}
			if cleared != 3 {
				t.Fatal("recovery cookies not cleared")
			}
		})
	}
}

func TestRecoveryScopeRejectsWrongSubjectAndMissingSession(t *testing.T) {
	token := recoveryTestToken(t, "recovery", 0)
	if _, ok := recoveryScope(token, "different", time.Now()); ok {
		t.Fatal("wrong subject accepted")
	}
	claims := jwt.MapClaims{}
	_, _, _ = jwt.NewParser().ParseUnverified(token, claims)
	delete(claims, "session_id")
	token, _ = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-only"))
	if _, ok := recoveryScope(token, sdkTestID, time.Now()); ok {
		t.Fatal("missing session accepted")
	}
}
