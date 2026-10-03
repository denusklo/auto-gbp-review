package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	supa "github.com/supabase-community/supabase-go"
)

var supabaseClient *supa.Client

var supabaseRecoveryHTTPClient = &http.Client{Timeout: time.Minute}

// InitSupabase initializes the Supabase client
func InitSupabase() error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseAnonKey := os.Getenv("SUPABASE_ANON_KEY")

	if supabaseURL == "" || supabaseAnonKey == "" {
		return fmt.Errorf("SUPABASE_URL and SUPABASE_ANON_KEY are required")
	}

	client, err := supa.NewClient(supabaseURL, supabaseAnonKey, nil)
	if err != nil {
		return fmt.Errorf("initialize Supabase client: %w", err)
	}

	// gotrue-go sets apikey but omits Authorization until a token is supplied.
	// Preserve the anonymous bearer header without storing any user's session.
	client.Auth = client.Auth.WithToken(supabaseAnonKey)
	supabaseClient = client
	return nil
}

// GetSupabaseClient returns the initialized Supabase client
func GetSupabaseClient() *supa.Client {
	return supabaseClient
}

// recoverSupabasePassword retains redirect_to, which gotrue-go Recover cannot set.
func recoverSupabasePassword(ctx context.Context, email, redirectURL string) error {
	body, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return err
	}
	endpoint := GetSupabaseURL() + "/auth/v1/recover"
	if redirectURL != "" {
		endpoint += "?" + url.Values{"redirect_to": {redirectURL}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	key := os.Getenv("SUPABASE_ANON_KEY")
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := supabaseRecoveryHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("password recovery status %d", resp.StatusCode)
		}
		return fmt.Errorf("password recovery status %d: %s", resp.StatusCode, body)
	}
	return nil
}

// GetSupabaseURL returns the Supabase project URL from environment
func GetSupabaseURL() string {
	return os.Getenv("SUPABASE_URL")
}

// GetSupabaseServiceKey returns the Supabase service role key from environment
func GetSupabaseServiceKey() string {
	return os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
}
