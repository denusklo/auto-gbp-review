package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/supabase-community/gotrue-go/types"
	supa "github.com/supabase-community/supabase-go"
)

const localSDKSmokeURL = "http://127.0.0.1:54321"

// TestSupabaseLocalIntegration is deliberately opt-in. It never starts the app,
// loads env files, or uses a project endpoint or key inherited from the shell.
func TestSupabaseLocalIntegration(t *testing.T) {
	if os.Getenv("RUN_LOCAL_SUPABASE_SMOKE") != "1" {
		t.Skip("SKIP local smoke opt-in")
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "SUPABASE_") {
			t.Setenv(name, "")
		}
	}
	statusBytes := localSmokeCommand(t, "credentials", "supabase", "status", "-o", "json")
	var status struct {
		APIURL         string `json:"API_URL"`
		AnonKey        string `json:"ANON_KEY"`
		ServiceRoleKey string `json:"SERVICE_ROLE_KEY"`
		InboxURL       string `json:"INBUCKET_URL"`
		MailpitURL     string `json:"MAILPIT_URL"`
	}
	if json.Unmarshal(statusBytes, &status) != nil || status.APIURL != localSDKSmokeURL || status.AnonKey == "" || status.ServiceRoleKey == "" {
		t.Fatal("FAIL local credentials validation")
	}
	t.Setenv("SUPABASE_URL", localSDKSmokeURL)
	t.Setenv("SUPABASE_ANON_KEY", status.AnonKey)
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", status.ServiceRoleKey)
	t.Log("PASS local credentials validation")

	// Confirm that recovery mail stays in this project's local inbox. The
	// template reads only the SMTP hostname and send-email hook enable flag.
	smtp := localSmokeCommand(t, "local SMTP validation", "docker", "inspect", "--format",
		`{{range .Config.Env}}{{$parts := split . "="}}{{if eq (index $parts 0) "GOTRUE_SMTP_HOST"}}SMTP={{index $parts 1}}{{println}}{{end}}{{if eq (index $parts 0) "GOTRUE_HOOK_SEND_EMAIL_ENABLED"}}HOOK={{index $parts 1}}{{println}}{{end}}{{end}}`,
		"supabase_auth_auto-gbp-review")
	smtpValues := map[string]string{}
	for _, field := range strings.Fields(string(smtp)) {
		name, value, _ := strings.Cut(field, "=")
		smtpValues[name] = value
	}
	if (status.InboxURL != "http://127.0.0.1:54324" && status.MailpitURL != "http://127.0.0.1:54324") || smtpValues["SMTP"] != "supabase_inbucket_auto-gbp-review" || (smtpValues["HOOK"] != "" && smtpValues["HOOK"] != "false") {
		t.Fatal("FAIL local SMTP validation")
	}
	t.Log("PASS local SMTP validation")

	runID := uuid.NewString()
	email := "sdk-smoke-" + runID + "@example.com"
	password := uuid.NewString() + "aA!9"
	newPassword := uuid.NewString() + "aA!9"
	transport := &localSmokeTransport{
		base: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != "127.0.0.1:54321" {
					return nil, errors.New("blocked non-local dial")
				}
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, address)
			},
		},
		email: email,
		runID: runID,
	}
	httpClient := http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("blocked redirect")
		},
	}
	client, err := supa.NewClient(localSDKSmokeURL, status.AnonKey, nil)
	if err != nil {
		t.Fatal("FAIL SDK initialization")
	}
	client.Auth = client.Auth.WithToken(status.AnonKey).WithClient(httpClient)
	admin := client.Auth.WithToken(status.ServiceRoleKey)
	// Register cleanup before signup, including a successful HTTP creation that
	// the SDK might fail to decode. Never list or delete unrelated accounts.
	t.Cleanup(func() {
		defer transport.base.CloseIdleConnections()
		id := transport.createdUserID()
		if id == uuid.Nil {
			if transport.creationAttempted() {
				t.Error("FAIL cleanup ownership unavailable")
			} else {
				t.Log("PASS cleanup no account created")
			}
			return
		}
		user, err := admin.AdminGetUser(types.AdminGetUserRequest{UserID: id})
		if err != nil || user.ID != id || user.Email != email || user.UserMetadata["sdk_smoke_run"] != runID {
			t.Error("FAIL cleanup ownership validation")
			return
		}
		if admin.AdminDeleteUser(types.AdminDeleteUserRequest{UserID: id}) != nil {
			t.Error("FAIL cleanup delete")
			return
		}
		req, err := http.NewRequest(http.MethodGet, localSDKSmokeURL+"/auth/v1/admin/users/"+id.String(), nil)
		if err != nil {
			t.Error("FAIL cleanup verification")
			return
		}
		req.Header.Set("apikey", status.AnonKey)
		req.Header.Set("Authorization", "Bearer "+status.ServiceRoleKey)
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Error("FAIL cleanup verification")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Error("FAIL cleanup verification")
			return
		}
		t.Log("PASS cleanup generated account deleted")
	})

	signedUp, err := client.Auth.Signup(types.SignupRequest{
		Email: email, Password: password,
		Data: map[string]interface{}{"role": "merchant", "sdk_smoke_run": runID},
	})
	if err != nil || signedUp.User.ID == uuid.Nil || signedUp.Email != email || signedUp.User.ID != transport.createdUserID() {
		t.Fatal("FAIL signup")
	}
	id := signedUp.User.ID
	t.Log("PASS signup")
	login, err := client.Auth.SignInWithEmailPassword(email, password)
	if err != nil || login.User.ID != id || login.AccessToken == "" || login.RefreshToken == "" {
		t.Fatal("FAIL login")
	}
	t.Log("PASS login")
	user, err := client.Auth.WithToken(login.AccessToken).GetUser()
	if err != nil || user.ID != id || user.Email != email {
		t.Fatal("FAIL get-user")
	}
	t.Log("PASS get-user")
	refreshed, err := client.Auth.WithToken(login.AccessToken).RefreshToken(login.RefreshToken)
	if err != nil || refreshed.User.ID != id || refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatal("FAIL refresh")
	}
	t.Log("PASS refresh")
	updated, err := client.Auth.WithToken(refreshed.AccessToken).UpdateUser(types.UpdateUserRequest{Password: &newPassword})
	if err != nil || updated.ID != id {
		t.Fatal("FAIL password-update")
	}
	t.Log("PASS password-update")
	relogin, err := client.Auth.SignInWithEmailPassword(email, newPassword)
	if err != nil || relogin.User.ID != id || relogin.AccessToken == "" || relogin.RefreshToken == "" {
		t.Fatal("FAIL relogin")
	}
	if _, err := client.Auth.SignInWithEmailPassword(email, password); !transport.authRejected(err, "password", "invalid_credentials") {
		t.Fatal("FAIL old-password rejection")
	}
	t.Log("PASS relogin and old-password rejection")

	// Exercise the application's recovery helper with the same guarded client.
	// Only a request is sent; no email contents or browser URLs are opened.
	oldRecoveryClient := supabaseRecoveryHTTPClient
	supabaseRecoveryHTTPClient = &httpClient
	recoveryErr := recoverSupabasePassword(context.Background(), email, localSDKSmokeURL+"/auth/v1/verify")
	supabaseRecoveryHTTPClient = oldRecoveryClient
	if recoveryErr != nil {
		t.Fatal("FAIL recovery")
	}
	t.Log("PASS recovery request to local inbox")
	// Actual GoTrue recovery sessions use AMR otp. Verify the real contract and
	// the handler directly, without starting an app or touching a browser daemon.
	link, err := admin.AdminGenerateLink(types.AdminGenerateLinkRequest{Type: types.LinkTypeRecovery, Email: email})
	if err != nil || link.HashedToken == "" {
		t.Fatal("FAIL local recovery proof generation")
	}
	verifyBody, _ := json.Marshal(map[string]string{"type": "recovery", "token_hash": link.HashedToken})
	verifyRequest, err := http.NewRequest(http.MethodPost, localSDKSmokeURL+"/auth/v1/verify", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatal("FAIL local recovery verification")
	}
	verifyRequest.Header.Set("Content-Type", "application/json")
	verifyRequest.Header.Set("apikey", status.AnonKey)
	verifyRequest.Header.Set("Authorization", "Bearer "+status.AnonKey)
	verifyResponse, err := httpClient.Do(verifyRequest)
	if err != nil {
		t.Fatal("FAIL local recovery verification")
	}
	var recovery types.Session
	decodeErr := json.NewDecoder(io.LimitReader(verifyResponse.Body, 1<<20)).Decode(&recovery)
	verifyResponse.Body.Close()
	if verifyResponse.StatusCode != http.StatusOK || decodeErr != nil || recovery.User.ID != id {
		t.Fatal("FAIL local recovery verification")
	}
	oldAuth := supabaseClient
	supabaseClient = client
	defer func() { supabaseClient = oldAuth }()
	t.Setenv("BASE_URL", "http://127.0.0.1:3000")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/recovery-session", RecoverySession)
	csrf := nonce()
	payload, _ := json.Marshal(map[string]string{"access_token": recovery.AccessToken, "refresh_token": recovery.RefreshToken, "csrf": csrf})
	bridgeRequest := httptest.NewRequest(http.MethodPost, "/auth/recovery-session", bytes.NewReader(payload))
	bridgeRequest.Header.Set("Origin", "http://127.0.0.1:3000")
	bridgeRequest.Header.Set("Content-Type", "application/json")
	bridgeRequest.AddCookie(&http.Cookie{Name: "recovery_bridge_csrf", Value: csrf})
	bridgeResponse := httptest.NewRecorder()
	router.ServeHTTP(bridgeResponse, bridgeRequest)
	if bridgeResponse.Code != http.StatusNoContent {
		t.Fatal("FAIL actual local OTP recovery-session")
	}
	t.Log("PASS actual local OTP recovery-session")
	if client.Auth.WithToken(relogin.AccessToken).Logout() != nil {
		t.Fatal("FAIL logout")
	}
	if _, err := client.Auth.RefreshToken(relogin.RefreshToken); !transport.authRejected(err, "refresh_token", "refresh_token_not_found") {
		t.Fatal("FAIL logout refresh revocation")
	}
	t.Log("PASS logout and refresh revocation")
	t.Log("LIMITATION inbox delivery and browser flow not verified")
}

// Capture CLI output in memory only. Never include command errors, stdout,
// stderr, SDK errors, or response bodies in test diagnostics.
func localSmokeCommand(t *testing.T, stage, program string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...)
	// Go runs package tests from internal/auth; resolve the CLI project at root.
	cmd.Dir = "../.."
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "SUPABASE_") || strings.HasSuffix(upper, "PROXY") || upper == "DOCKER_HOST" || upper == "DOCKER_CONTEXT" {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "DOCKER_HOST=unix:///var/run/docker.sock", "DOCKER_CONTEXT=default")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("FAIL " + stage)
	}
	return output
}

type localSmokeTransport struct {
	base         *http.Transport
	email, runID string
	mu           sync.Mutex
	createdID    uuid.UUID
	attempted    bool
	tokenResult  localSmokeTokenResult
}

// Keep only non-sensitive machine fields. The SDK's generic error contains the
// full body, so assertions must never format or log that error.
type localSmokeTokenResult struct {
	status      int
	grant, code string
}

func (transport *localSmokeTransport) authRejected(err error, grant, code string) bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	var networkError net.Error
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		!errors.As(err, &networkError) && transport.tokenResult.status == http.StatusBadRequest &&
		transport.tokenResult.grant == grant && transport.tokenResult.code == code
}

func (transport *localSmokeTransport) createdUserID() uuid.UUID {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.createdID
}

func (transport *localSmokeTransport) creationAttempted() bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.attempted
}

func (transport *localSmokeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "http" || req.URL.Host != "127.0.0.1:54321" || req.URL.User != nil || req.URL.Fragment != "" || req.Host != "127.0.0.1:54321" {
		return nil, errors.New("blocked non-local request")
	}
	allowed := false
	switch req.URL.Path {
	case "/auth/v1/signup", "/auth/v1/token", "/auth/v1/recover", "/auth/v1/logout", "/auth/v1/verify", "/auth/v1/admin/generate_link":
		allowed = req.Method == http.MethodPost
	case "/auth/v1/user":
		allowed = req.Method == http.MethodGet || req.Method == http.MethodPut
	default:
		id := transport.createdUserID()
		allowed = id != uuid.Nil && req.URL.Path == "/auth/v1/admin/users/"+id.String() && (req.Method == http.MethodGet || req.Method == http.MethodDelete)
	}
	if !allowed {
		return nil, errors.New("blocked non-smoke endpoint")
	}
	if req.URL.Path == "/auth/v1/recover" {
		redirect, err := url.Parse(req.URL.Query().Get("redirect_to"))
		if err != nil || redirect.Scheme != "http" || redirect.Host != "127.0.0.1:54321" {
			return nil, errors.New("blocked recovery redirect")
		}
	}
	if req.URL.Path == "/auth/v1/signup" {
		transport.mu.Lock()
		transport.attempted = true
		transport.mu.Unlock()
	}
	if req.URL.Path == "/auth/v1/token" {
		transport.mu.Lock()
		transport.tokenResult = localSmokeTokenResult{}
		transport.mu.Unlock()
	}
	resp, err := transport.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if req.URL.Path == "/auth/v1/token" {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, errors.New("token response unavailable")
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		var result struct {
			Code string `json:"error_code"`
		}
		if json.Unmarshal(body, &result) == nil {
			transport.mu.Lock()
			transport.tokenResult = localSmokeTokenResult{
				status: resp.StatusCode, grant: req.URL.Query().Get("grant_type"), code: result.Code,
			}
			transport.mu.Unlock()
		}
		return resp, nil
	}
	if req.URL.Path != "/auth/v1/signup" || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return nil, errors.New("signup response unavailable")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var result struct {
		ID       uuid.UUID              `json:"id"`
		Email    string                 `json:"email"`
		Metadata map[string]interface{} `json:"user_metadata"`
		User     struct {
			ID       uuid.UUID              `json:"id"`
			Email    string                 `json:"email"`
			Metadata map[string]interface{} `json:"user_metadata"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &result) == nil {
		if result.User.ID != uuid.Nil {
			result.ID, result.Email, result.Metadata = result.User.ID, result.User.Email, result.User.Metadata
		}
		if result.ID != uuid.Nil && result.Email == transport.email && result.Metadata["sdk_smoke_run"] == transport.runID {
			transport.mu.Lock()
			transport.createdID = result.ID
			transport.mu.Unlock()
		}
	}
	return resp, nil
}

func TestLocalSmokeAuthRejectionClassification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		grant, code string
		err         error
		want        bool
	}{
		{"invalid password", 400, "password", "invalid_credentials", errors.New("SDK HTTP rejection"), true},
		{"revoked refresh", 400, "refresh_token", "refresh_token_not_found", errors.New("SDK HTTP rejection"), true},
		{"timeout", 400, "password", "invalid_credentials", context.DeadlineExceeded, false},
		{"cancellation", 400, "password", "invalid_credentials", context.Canceled, false},
		{"transport", 400, "password", "invalid_credentials", &url.Error{Op: "Post", Err: errors.New("transport failure")}, false},
		{"no response", 0, "password", "", errors.New("transport failure"), false},
		{"server error", 503, "password", "invalid_credentials", errors.New("SDK HTTP rejection"), false},
		{"rate limited", 429, "password", "invalid_credentials", errors.New("SDK HTTP rejection"), false},
		{"wrong code", 400, "password", "unexpected_failure", errors.New("SDK HTTP rejection"), false},
		{"wrong grant", 400, "refresh_token", "invalid_credentials", errors.New("SDK HTTP rejection"), false},
		{"no SDK error", 400, "password", "invalid_credentials", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &localSmokeTransport{tokenResult: localSmokeTokenResult{status: tc.status, grant: tc.grant, code: tc.code}}
			grant, code := "password", "invalid_credentials"
			if tc.name == "revoked refresh" {
				grant, code = "refresh_token", "refresh_token_not_found"
			}
			if transport.authRejected(tc.err, grant, code) != tc.want {
				t.Error("FAIL auth rejection classification")
			}
		})
	}
}
