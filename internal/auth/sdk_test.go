package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/supabase-community/gotrue-go/types"
	supa "github.com/supabase-community/supabase-go"
)

const sdkTestID = "12345678-1234-1234-1234-123456789abc"

func TestSupabaseSDKContracts(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		if r.Header.Get("apikey") != "test-anon" {
			t.Error("missing API key")
		}
		var body map[string]interface{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch r.URL.Path {
		case "/auth/v1/token":
			if r.Header.Get("Authorization") != "Bearer test-anon" {
				t.Error("singleton auth changed")
			}
			switch r.URL.Query().Get("grant_type") {
			case "password":
				if body["email"] != "test@example.com" || body["password"] != "test-password" {
					t.Error("login payload changed")
				}
			case "refresh_token":
				if body["refresh_token"] != "test-refresh" {
					t.Error("refresh payload changed")
				}
			default:
				t.Error("missing grant type")
			}
			fmt.Fprintf(w, `{"access_token":"test-session","refresh_token":"next-refresh","user":{"id":%q,"email":"test@example.com"}}`, sdkTestID)
		case "/auth/v1/signup":
			if body["data"].(map[string]interface{})["role"] != "merchant" {
				t.Error("signup role changed")
			}
			if body["email"] == "duplicate@example.com" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"msg":"User already registered"}`)
				return
			}
			fmt.Fprintf(w, `{"id":%q}`, sdkTestID)
		case "/auth/v1/user":
			token := r.Header.Get("Authorization")
			if token != "Bearer first-user" && token != "Bearer second-user" {
				t.Errorf("wrong user token %q", token)
			}
			if r.Method == http.MethodPut && body["password"] != "new-password" {
				t.Error("password update changed")
			}
			fmt.Fprintf(w, `{"id":%q,"email":"test@example.com"}`, sdkTestID)
		case "/auth/v1/logout":
			if r.Header.Get("Authorization") != "Bearer first-user" {
				t.Error("logout token changed")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv("SUPABASE_URL", server.URL)
	t.Setenv("SUPABASE_ANON_KEY", "test-anon")
	old := supabaseClient
	defer func() { supabaseClient = old }()
	if err := InitSupabase(); err != nil {
		t.Fatal(err)
	}
	client := GetSupabaseClient()
	login, err := client.Auth.SignInWithEmailPassword("test@example.com", "test-password")
	if err != nil || login.AccessToken != "test-session" {
		t.Fatalf("login: %v %v", login, err)
	}
	_, err = client.Auth.Signup(types.SignupRequest{Email: "test@example.com", Password: "test-password", Data: map[string]interface{}{"role": "merchant"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Auth.Signup(types.SignupRequest{Email: "duplicate@example.com", Data: map[string]interface{}{"role": "merchant"}})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("registration error: %v", err)
	}
	refreshed, err := client.Auth.RefreshToken("test-refresh")
	if err != nil || refreshed.RefreshToken != "next-refresh" {
		t.Fatalf("refresh: %v %v", refreshed, err)
	}
	var wg sync.WaitGroup
	for _, token := range []string{"first-user", "second-user"} {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			user, err := client.Auth.WithToken(token).GetUser()
			if err != nil || user.ID.String() != sdkTestID {
				t.Errorf("user: %v %v", user, err)
			}
		}(token)
	}
	wg.Wait()
	password := "new-password"
	if _, err := client.Auth.WithToken("first-user").UpdateUser(types.UpdateUserRequest{Password: &password}); err != nil {
		t.Fatal(err)
	}
	if err := client.Auth.WithToken("first-user").Logout(); err != nil {
		t.Fatal(err)
	}
	// Sign-in and refresh must not replace the shared anonymous credentials.
	if _, err := client.Auth.SignInWithEmailPassword("test@example.com", "test-password"); err != nil {
		t.Fatal(err)
	}
	if seen["GET /auth/v1/user"] != 2 {
		t.Error("missing concurrent user requests")
	}
}

func TestSupabaseRecoveryRedirect(t *testing.T) {
	redirect := "https://example.com/auth/callback?flow=recovery&next=/dashboard"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/recover" || r.URL.Query().Get("redirect_to") != redirect || len(r.URL.Query()) != 1 {
			t.Errorf("recovery URL %s", r.URL)
		}
		if r.Header.Get("apikey") != "test-anon" || r.Header.Get("Authorization") != "Bearer test-anon" {
			t.Error("recovery headers changed")
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["email"] == "fail@example.com" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"msg":"recovery failed"}`)
			return
		}
		if body["email"] != "test@example.com" {
			t.Error("recovery email changed")
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	t.Setenv("SUPABASE_URL", server.URL)
	t.Setenv("SUPABASE_ANON_KEY", "test-anon")
	if err := recoverSupabasePassword(context.Background(), "test@example.com", redirect); err != nil {
		t.Fatal(err)
	}
	if err := recoverSupabasePassword(context.Background(), "fail@example.com", redirect); err == nil || !strings.Contains(err.Error(), "recovery failed") {
		t.Fatalf("recovery error %v", err)
	}
}

func TestSupabaseCallbackSessionContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ kind, wireType, location, cookie string }{
		{"signup", "email", "/dashboard?verified=true", "sb_access_token"},
		{"recovery", "recovery", "/reset-password?flow=recovery", "reset_access_token"},
		{"email_change", "email_change", "/dashboard?email_changed=true", "sb_access_token"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			access := "verified-session"
			if tc.kind == "recovery" {
				access = recoveryTestToken(t, "otp", 0)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/auth/v1/user" {
					if r.Header.Get("Authorization") != "Bearer "+access {
						t.Error("online access verification missing")
					}
					fmt.Fprintf(w, `{"id":%q,"email":"test@example.com"}`, sdkTestID)
					return
				}
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/verify" || body["type"] != tc.wireType || body["token_hash"] != strings.Repeat("a", 64) {
					t.Error("verification contract changed")
				}
				if r.Header.Get("apikey") != "test-anon" || r.Header.Get("Authorization") != "Bearer test-anon" {
					t.Error("verification headers changed")
				}
				fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"verified-refresh","user":{"id":%q,"email":"test@example.com"}}`, access, sdkTestID)
			}))
			defer server.Close()
			t.Setenv("SUPABASE_URL", server.URL)
			t.Setenv("SUPABASE_ANON_KEY", "test-anon")
			old := supabaseClient
			defer func() { supabaseClient = old }()
			if err := InitSupabase(); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.GET("/callback", HandleSupabaseAuthCallback)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/callback?token_hash="+strings.Repeat("a", 64)+"&type="+tc.kind, nil))
			if tc.kind == "recovery" {
				if rec.Code != http.StatusFound || rec.Header().Get("Location") != tc.location {
					t.Fatalf("callback redirect: %d %s", rec.Code, rec.Header().Get("Location"))
				}
			} else {
				body := rec.Body.String()
				if rec.Code != http.StatusOK || rec.Header().Get("Location") != "" || !strings.Contains(body, tc.location) || !strings.Contains(body, "history.replaceState") || !strings.Contains(body, "window.location.replace") {
					t.Fatal("callback must commit a same-site landing document")
				}
			}
			found := false
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == tc.cookie && cookie.Value == access && cookie.HttpOnly && cookie.SameSite == http.SameSiteLaxMode {
					found = true
				}
			}
			if !found {
				t.Error("missing verification cookie")
			}
		})
	}
}

func TestSupabaseMiddlewareRefreshAndStringID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_role": "merchant"}).SignedString([]byte("test-only"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"msg":"expired"}`)
		case "/auth/v1/token":
			fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"rotated","user":{"id":%q,"email":"test@example.com"}}`, token, sdkTestID)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	old := supabaseClient
	defer func() { supabaseClient = old }()
	supabaseClient, err = supa.NewClient(server.URL, "test-anon", nil)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/private", SupabaseAuthMiddleware("merchant"), func(c *gin.Context) {
		id, ok := c.Get("user_id")
		if !ok || id != sdkTestID {
			t.Errorf("user_id must stay string: %T %v", id, id)
		}
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: "sb_access_token", Value: url.QueryEscape(token)})
	req.AddCookie(&http.Cookie{Name: "sb_refresh_token", Value: "test-refresh"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("refresh rejected: %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 || cookies[1].Value != "rotated" {
		t.Fatalf("refresh cookies: %v", cookies)
	}
	// A safe top-level GET returning from an OAuth provider needs these cookies.
	for _, cookie := range cookies {
		if cookie.SameSite != http.SameSiteLaxMode || !cookie.HttpOnly {
			t.Fatal("refreshed session cookies must preserve OAuth-return Lax behavior")
		}
	}
}

func TestSupabaseMiddlewareUsesRefreshedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, oldRole, newRole, requiredRole string
		allowed                              bool
	}{
		{"demoted admin", "admin", "merchant", "admin", false},
		{"promoted merchant", "merchant", "admin", "admin", true},
		{"demoted superadmin", "superadmin", "admin", "superadmin", false},
		{"updated context role", "admin", "merchant", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sign := func(role string, expiry int64) string {
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"user_role": role, "exp": expiry,
				}).SignedString([]byte("test-only"))
				if err != nil {
					t.Fatal(err)
				}
				return token
			}
			oldToken := sign(tc.oldRole, 1)
			newToken := sign(tc.newRole, 4102444800)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+oldToken {
					t.Error("validation/refresh must use the request's original token")
				}
				switch r.URL.Path {
				case "/auth/v1/user":
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"msg":"expired"}`)
				case "/auth/v1/token":
					if r.URL.Query().Get("grant_type") != "refresh_token" {
						t.Error("wrong grant")
					}
					fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"rotated","user":{"id":%q,"email":"test@example.com"}}`, newToken, sdkTestID)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			oldClient := supabaseClient
			defer func() { supabaseClient = oldClient }()
			var err error
			supabaseClient, err = supa.NewClient(server.URL, "test-anon", nil)
			if err != nil {
				t.Fatal(err)
			}
			reached := false
			router := gin.New()
			router.GET("/private", SupabaseAuthMiddleware(tc.requiredRole), func(c *gin.Context) {
				reached = true
				if role := c.GetString("user_role"); role != tc.newRole {
					t.Errorf("context role = %q, want refreshed %q", role, tc.newRole)
				}
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/private", nil)
			req.AddCookie(&http.Cookie{Name: "sb_access_token", Value: oldToken})
			req.AddCookie(&http.Cookie{Name: "sb_refresh_token", Value: "test-refresh"})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if reached != tc.allowed {
				t.Fatalf("handler reached = %v, want %v", reached, tc.allowed)
			}
			if tc.allowed && rec.Code != http.StatusNoContent {
				t.Errorf("status = %d", rec.Code)
			}
			if !tc.allowed && (rec.Code == http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "Access denied")) {
				t.Error("denied route must render an access-denied page")
			}
			found := false
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == "sb_access_token" && cookie.Value == newToken {
					found = true
				}
			}
			if !found {
				t.Error("missing refreshed access cookie")
			}
		})
	}
}

type sdkWaitingTransport struct {
	started chan context.Context
}

func (transport sdkWaitingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport.started <- req.Context()
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestSupabaseRecoveryTimeoutAndCancellation(t *testing.T) {
	if supabaseRecoveryHTTPClient.Timeout != time.Minute {
		t.Fatalf("recovery timeout = %v, want old SDK's one minute", supabaseRecoveryHTTPClient.Timeout)
	}
	// This transport never opens a socket. Client timeout and caller cancellation
	// release it through the actual HTTP request context, without server sleeps.
	for _, timeout := range []bool{false, true} {
		name := "caller cancellation"
		if timeout {
			name = "client timeout"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("SUPABASE_URL", "http://sdk-test.invalid")
			t.Setenv("SUPABASE_ANON_KEY", "test-anon")
			started := make(chan context.Context, 1)
			oldClient := supabaseRecoveryHTTPClient
			client := *oldClient
			client.Transport = sdkWaitingTransport{started: started}
			if timeout {
				client.Timeout = time.Millisecond
			}
			supabaseRecoveryHTTPClient = &client
			defer func() { supabaseRecoveryHTTPClient = oldClient }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- recoverSupabasePassword(ctx, "test@example.com", "") }()
			select {
			case requestCtx := <-started:
				if _, bounded := requestCtx.Deadline(); !bounded {
					t.Error("request lacks client deadline")
				}
			case <-time.After(time.Second):
				cancel()
				<-result
				t.Fatal("recovery did not start")
			}
			if !timeout {
				cancel()
			}
			select {
			case err := <-result:
				if timeout {
					var netErr net.Error
					if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &netErr) || !netErr.Timeout() {
						t.Errorf("expected timeout, got %v", err)
					}
				} else if !errors.Is(err, context.Canceled) {
					t.Errorf("expected caller cancellation, got %v", err)
				}
			case <-time.After(time.Second):
				cancel()
				<-result
				t.Fatal("recovery did not stop")
			}
		})
	}
}
