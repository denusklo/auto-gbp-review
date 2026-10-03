package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/supabase-community/gotrue-go/types"
)

const recoveryLifetime = 600

func validBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}

func sameOrigin(c *gin.Context) bool {
	base := strings.TrimRight(os.Getenv("BASE_URL"), "/")
	return validBaseURL(base) && c.GetHeader("Origin") == base
}

func nonce() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func authCookie(c *gin.Context, name, value string, age int) {
	base := os.Getenv("BASE_URL")
	configured, _ := url.Parse(base)
	// Reverse-proxy deployments use the configured public scheme, never an
	// untrusted Forwarded header. Missing/invalid configuration fails secure.
	secure := !validBaseURL(base) || configured.Scheme == "https" || c.Request.TLS != nil
	sameSite := http.SameSiteStrictMode
	// Preserve the original browser-default Lax session behavior for top-level
	// OAuth/email GET returns. State-changing auth/recovery POST gates remain.
	// The fragment bridge's CSRF cookie is deliberately Strict, never None.
	if name == "sb_access_token" || name == "sb_refresh_token" || name == "auth_token" || name == "reset_access_token" || name == "reset_csrf" {
		sameSite = http.SameSiteLaxMode
	}
	c.SetSameSite(sameSite)
	c.SetCookie(name, value, age, "/", "", secure, true)
}

func securityHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
}

// HandleSupabaseAuthCallback accepts existing token-hash links or serves a
// first-party fragment bridge. No fragment token enters server URLs or logs.
func HandleSupabaseAuthCallback(c *gin.Context) {
	securityHeaders(c)
	hash, kind := c.Query("token_hash"), c.Query("type")
	if hash == "" && kind == "" {
		n := nonce()
		if n == "" {
			c.Status(500)
			return
		}
		authCookie(c, "recovery_bridge_csrf", n, recoveryLifetime)
		c.Header("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+n+"'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		c.Header("Content-Type", "text/html; charset=utf-8")
		_ = bridgeTemplate.Execute(c.Writer, n)
		return
	}
	wireKind := kind
	if kind == "signup" {
		wireKind = "email"
	}
	if len(hash) < 40 || len(hash) > 256 || (kind != "signup" && kind != "recovery" && kind != "email_change") {
		c.String(http.StatusBadRequest, "Invalid authentication link")
		return
	}
	body, _ := json.Marshal(map[string]string{"token_hash": hash, "type": wireKind})
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, GetSupabaseURL()+"/auth/v1/verify", bytes.NewReader(body))
	if err != nil {
		c.Status(400)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	key := os.Getenv("SUPABASE_ANON_KEY")
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := supabaseRecoveryHTTPClient.Do(req)
	if err != nil {
		c.String(400, "Authentication verification failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.String(400, "Invalid or expired authentication link")
		return
	}
	var session types.Session
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&session) != nil {
		c.Status(400)
		return
	}
	if kind == "recovery" {
		if session.AccessToken == "" {
			c.Status(400)
			return
		}
		age, valid := verifiedRecoveryAge(session.AccessToken)
		if !valid {
			c.Status(http.StatusUnauthorized)
			return
		}
		setRecoverySession(c, session.AccessToken, age)
		return
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		c.String(http.StatusBadRequest, "Authentication session unavailable. Please sign in again.")
		return
	}
	n := nonce()
	if n == "" {
		c.Status(http.StatusInternalServerError)
		return
	}
	authCookie(c, "sb_access_token", session.AccessToken, 3600)
	authCookie(c, "sb_refresh_token", session.RefreshToken, 86400*7)
	destination := "/dashboard?verified=true"
	if kind == "email_change" {
		destination = "/dashboard?email_changed=true"
	}
	// Commit a first-party document to scrub callback credentials before fixed
	// same-origin navigation. Neither bearer values nor caller-provided redirect
	// targets enter this page.
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+n+"'; base-uri 'none'; frame-ancestors 'none'")
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = verifiedLandingTemplate.Execute(c.Writer, struct{ Nonce, Destination string }{n, destination})
}

var verifiedLandingTemplate = template.Must(template.New("verified-landing").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Authentication complete</title><script nonce="{{.Nonce}}">
history.replaceState(null, '', '/auth/callback');
window.location.replace({{.Destination}});
</script></head><body>Authentication complete. <a href="{{.Destination}}">Continue to your dashboard</a>.</body></html>`))

var bridgeTemplate = template.Must(template.New("bridge").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Account recovery</title><script nonce="{{.}}">
(async () => {
 const fragment = new URLSearchParams(window.location.hash.slice(1));
 history.replaceState(null, '', '/auth/callback');
 const access = fragment.get('access_token');
 const refresh = fragment.get('refresh_token');
 if (fragment.get('type') !== 'recovery' || !access || !refresh) {
  document.addEventListener('DOMContentLoaded', () => { document.body.textContent = 'Invalid recovery link.'; }); return;
 }
 try {
  const response = await fetch('/auth/recovery-session', {method:'POST', credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:JSON.stringify({access_token:access, refresh_token:refresh, csrf:{{.}}})});
  if (response.ok) { window.location.replace('/reset-password?flow=recovery'); }
  else { document.body.textContent = 'Invalid or expired recovery link.'; }
 } catch (_) { document.body.textContent = 'Recovery verification failed.'; }
})();
</script></head><body>Verifying recovery link...</body></html>`))

// RecoverySession verifies access online before inspecting recent email-OTP
// authentication. GoTrue uses "otp" for recovery and email sign-in alike; the
// client fragment type only selects UI routing and is never authentication proof.
// This authorizes a password update for that same authenticated account, not a
// claim that an OTP token exclusively originated from a recovery email.
func RecoverySession(c *gin.Context) {
	securityHeaders(c)
	if !sameOrigin(c) {
		c.Status(http.StatusForbidden)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		CSRF         string `json:"csrf"`
	}
	if c.ShouldBindJSON(&payload) != nil || payload.AccessToken == "" || payload.RefreshToken == "" || !csrfMatches(c, "recovery_bridge_csrf", payload.CSRF) {
		c.Status(400)
		return
	}
	age, ok := verifiedRecoveryAge(payload.AccessToken)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	authCookie(c, "recovery_bridge_csrf", "", -1)
	setRecoveryCookies(c, payload.AccessToken, age)
	c.Status(http.StatusNoContent)
}

// verifiedRecoveryAge must perform the online check first. Parsing signed JWT
// fields without verification is safe only after Supabase has accepted access.
func verifiedRecoveryAge(token string) (int, bool) {
	user, err := GetSupabaseClient().Auth.WithToken(token).GetUser()
	if err != nil {
		return 0, false
	}
	return recoveryScope(token, user.ID.String(), time.Now())
}

func recoveryScope(token, userID string, now time.Time) (int, bool) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return 0, false
	}
	sub, _ := claims["sub"].(string)
	sid, _ := claims["session_id"].(string)
	exp, e1 := claims.GetExpirationTime()
	issued, e2 := claims.GetIssuedAt()
	if e1 != nil || e2 != nil || exp == nil || issued == nil || sub != userID || sid == "" || !exp.After(now) || issued.After(now.Add(30*time.Second)) || issued.Before(now.Add(-recoveryLifetime*time.Second)) {
		return 0, false
	}
	methods, _ := claims["amr"].([]interface{})
	for _, value := range methods {
		method, _ := value.(map[string]interface{})
		timestamp, _ := method["timestamp"].(float64)
		if (method["method"] != "otp" && method["method"] != "recovery") || timestamp <= 0 {
			continue
		}
		verifiedAt := time.Unix(int64(timestamp), 0)
		if verifiedAt.After(now.Add(30 * time.Second)) {
			continue
		}
		deadline := verifiedAt.Add(recoveryLifetime * time.Second)
		if exp.Before(deadline) {
			deadline = exp.Time
		}
		age := int(deadline.Sub(now).Seconds())
		if age > 0 && age <= recoveryLifetime {
			return age, true
		}
	}
	return 0, false
}

func csrfMatches(c *gin.Context, cookie, supplied string) bool {
	expected, err := c.Cookie(cookie)
	return err == nil && len(expected) >= 32 && subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

func setRecoveryCookies(c *gin.Context, token string, age int) {
	authCookie(c, "reset_access_token", token, age)
	authCookie(c, "reset_csrf", nonce(), age)
}

func setRecoverySession(c *gin.Context, token string, age int) {
	setRecoveryCookies(c, token, age)
	c.Redirect(http.StatusFound, "/reset-password?flow=recovery")
}

func clearRecovery(c *gin.Context) {
	for _, name := range []string{"reset_access_token", "reset_csrf", "recovery_bridge_csrf"} {
		authCookie(c, name, "", -1)
	}
}

func ResetPasswordCallback(c *gin.Context) {
	securityHeaders(c)
	if !sameOrigin(c) || !csrfMatches(c, "reset_csrf", c.PostForm("csrf")) {
		c.Status(http.StatusForbidden)
		return
	}
	token, err := c.Cookie("reset_access_token")
	if err != nil || token == "" {
		c.Redirect(302, "/forgot-password?error=session_expired")
		return
	}
	if _, valid := verifiedRecoveryAge(token); !valid {
		clearRecovery(c)
		c.Status(http.StatusUnauthorized)
		return
	}
	password := c.PostForm("password")
	if len(password) < 6 || password != c.PostForm("confirm_password") {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{"error": "Passwords must match and contain at least 6 characters", "csrf": c.PostForm("csrf")})
		return
	}
	if _, err := GetSupabaseClient().Auth.WithToken(token).UpdateUser(types.UpdateUserRequest{Password: &password}); err != nil {
		clearRecovery(c)
		c.String(400, "Password reset failed. Request a new recovery link.")
		return
	}
	clearRecovery(c)
	c.Redirect(302, "/login?password_reset=true")
}
