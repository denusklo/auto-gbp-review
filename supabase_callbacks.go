package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	supa "github.com/nedpals/supabase-go"
)

// HandleSupabaseAuthCallback handles Supabase auth redirects
func HandleSupabaseAuthCallback(c *gin.Context) {
	// Get query parameters from Supabase
	tokenHash := c.Query("token_hash")
	tokenType := c.Query("type")
	redirectTo := c.Query("redirect_to")

	log.Printf("Auth callback received: token_hash=%s, type=%s, redirect_to=%s",
		tokenHash, tokenType, redirectTo)

	if tokenHash == "" || tokenType == "" {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Invalid authentication link. Please try again.",
			"title": "Authentication Error",
		})
		return
	}

	// Validate token hash format (should be 64 hex chars for SHA256)
	if len(tokenHash) < 40 {
		log.Printf("Invalid token hash length: %d (expected at least 40 chars)", len(tokenHash))
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Invalid authentication link format. Please request a new verification email.",
			"title": "Authentication Error",
		})
		return
	}

	// Verify the token with Supabase
	client := GetSupabaseClient()
	ctx := context.Background()

	var resp *supa.AuthenticatedDetails

	log.Printf("Attempting to verify OTP:")
	log.Printf("  - TokenHash: %s (length: %d)", tokenHash, len(tokenHash))
	log.Printf("  - Type: %s", tokenType)
	log.Printf("  - RedirectTo: %s", redirectTo)
	log.Printf("  - Supabase URL from env: %s", client.BaseURL)

	// Use different verification methods based on the verification type
	switch tokenType {
	case "signup":
		// Using nedpals/supabase-go library (with known issue - we'll fix it)
		log.Printf("Using nedpals/supabase-go library for signup verification")

		// Bug fix: The library's VerifyOtp with TokenHashOtpCredentials fails
		// We need to make a direct HTTP request until the library is fixed
		// This is a workaround that works with Supabase's API
		verifyURL := fmt.Sprintf("%s/auth/v1/verify", client.BaseURL)

		requestBody := map[string]string{
			"token_hash": tokenHash,
			"type":       "email",
		}

		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			log.Printf("Error marshaling request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to process verification request.",
				"title": "Authentication Error",
			})
			return
		}

		log.Printf("Making direct HTTP request to: %s", verifyURL)
		log.Printf("Request body: %s", string(jsonBody))

		req, err := http.NewRequestWithContext(ctx, "POST", verifyURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			log.Printf("Error creating request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to create verification request.",
				"title": "Authentication Error",
			})
			return
		}

		// Set headers (need both apikey and Authorization for Supabase)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("apikey", os.Getenv("SUPABASE_ANON_KEY"))
		req.Header.Set("Authorization", "Bearer "+os.Getenv("SUPABASE_ANON_KEY"))

		httpClient := &http.Client{}
		httpResp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Error making request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to verify with Supabase.",
				"title": "Authentication Error",
			})
			return
		}
		defer httpResp.Body.Close()

		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			log.Printf("Error reading response: %v", err)
		}

		log.Printf("Response status: %d", httpResp.StatusCode)
		log.Printf("Response body: %s", string(respBody))

		if httpResp.StatusCode != 200 {
			log.Printf("Verification failed with status %d", httpResp.StatusCode)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Invalid or expired authentication link.",
				"title": "Authentication Error",
			})
			return
		}

		// Parse the response
		var authDetails supa.AuthenticatedDetails
		if err := json.Unmarshal(respBody, &authDetails); err != nil {
			log.Printf("Error parsing response: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to process verification response.",
				"title": "Authentication Error",
			})
			return
		}

		resp = &authDetails

	case "recovery":
		// For password recovery, use direct HTTP request workaround
		log.Printf("Using direct HTTP request for recovery verification")

		verifyURL := fmt.Sprintf("%s/auth/v1/verify", client.BaseURL)

		requestBody := map[string]string{
			"token_hash": tokenHash,
			"type":       "recovery",
		}

		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			log.Printf("Error marshaling recovery request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to process recovery request.",
				"title": "Authentication Error",
			})
			return
		}

		log.Printf("Making recovery HTTP request to: %s", verifyURL)

		req, err := http.NewRequestWithContext(ctx, "POST", verifyURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			log.Printf("Error creating recovery request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to create recovery request.",
				"title": "Authentication Error",
			})
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("apikey", os.Getenv("SUPABASE_ANON_KEY"))
		req.Header.Set("Authorization", "Bearer "+os.Getenv("SUPABASE_ANON_KEY"))

		httpClient := &http.Client{}
		httpResp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Error making recovery request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to verify with Supabase.",
				"title": "Authentication Error",
			})
			return
		}
		defer httpResp.Body.Close()

		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			log.Printf("Error reading recovery response: %v", err)
		}

		log.Printf("Recovery response status: %d", httpResp.StatusCode)
		log.Printf("Recovery response body: %s", string(respBody))

		if httpResp.StatusCode != 200 {
			log.Printf("Recovery verification failed with status %d", httpResp.StatusCode)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Invalid or expired recovery link.",
				"title": "Authentication Error",
			})
			return
		}

		// Parse the recovery response
		var authDetails supa.AuthenticatedDetails
		if err := json.Unmarshal(respBody, &authDetails); err != nil {
			log.Printf("Error parsing recovery response: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to process recovery response.",
				"title": "Authentication Error",
			})
			return
		}

		// Store the access token for password reset and redirect to reset page
		c.SetCookie("reset_access_token", authDetails.AccessToken, 600, "/", "", false, true)
		c.Redirect(http.StatusFound, "/reset-password?flow=recovery")
		log.Printf("Password recovery initiated for: %s", authDetails.User.Email)
		return

	case "email_change":
		// For email change verification, use direct HTTP request workaround
		log.Printf("Using direct HTTP request for email change verification")

		verifyURL := fmt.Sprintf("%s/auth/v1/verify", client.BaseURL)

		requestBody := map[string]string{
			"token_hash": tokenHash,
			"type":       "email_change",
		}

		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			log.Printf("Error marshaling email change request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to process email change request.",
				"title": "Authentication Error",
			})
			return
		}

		log.Printf("Making email change HTTP request to: %s", verifyURL)

		req, err := http.NewRequestWithContext(ctx, "POST", verifyURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			log.Printf("Error creating email change request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to create email change request.",
				"title": "Authentication Error",
			})
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("apikey", os.Getenv("SUPABASE_ANON_KEY"))
		req.Header.Set("Authorization", "Bearer "+os.Getenv("SUPABASE_ANON_KEY"))

		httpClient := &http.Client{}
		httpResp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Error making email change request: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to verify with Supabase.",
				"title": "Authentication Error",
			})
			return
		}
		defer httpResp.Body.Close()

		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			log.Printf("Error reading email change response: %v", err)
		}

		log.Printf("Email change response status: %d", httpResp.StatusCode)
		log.Printf("Email change response body: %s", string(respBody))

		if httpResp.StatusCode != 200 {
			log.Printf("Email change verification failed with status %d", httpResp.StatusCode)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Invalid or expired email change link.",
				"title": "Authentication Error",
			})
			return
		}

		// Parse the email change response
		var authDetails supa.AuthenticatedDetails
		if err := json.Unmarshal(respBody, &authDetails); err != nil {
			log.Printf("Error parsing email change response: %v", err)
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Failed to process email change response.",
				"title": "Authentication Error",
			})
			return
		}

		resp = &authDetails

	default:
		// Unknown verification type
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Unknown authentication type.",
			"title": "Authentication Error",
		})
		return
	}

	// Only continue if we have a valid response (signup or email_change cases)
	if resp == nil {
		// Already handled errors in the switch statement
		return
	}

	// Get user email from the response
	userEmail := resp.User.Email

	// Set authentication cookies for successful verification
	if resp.AccessToken != "" {
		c.SetCookie("sb_access_token", resp.AccessToken, 3600, "/", "", false, true)
		c.SetCookie("sb_refresh_token", resp.RefreshToken, 86400*7, "/", "", false, true)
	}

	// Handle different auth types
	switch tokenType {
	case "signup", "email":
		// Email verification successful - redirect to dashboard
		c.Redirect(http.StatusFound, "/dashboard?verified=true")
		log.Printf("Email verified for: %s", userEmail)

	case "email_change":
		// Email change confirmation
		c.Redirect(http.StatusFound, "/dashboard?email_changed=true")
		log.Printf("Email changed for: %s", userEmail)

	default:
		log.Printf("Unhandled auth type in success flow: %s", tokenType)
		c.Redirect(http.StatusFound, "/dashboard")
	}
}

// ResetPasswordCallback handles the actual password reset form submission
func ResetPasswordCallback(c *gin.Context) {
	// Get the access token stored during recovery flow
	accessToken, _ := c.Cookie("reset_access_token")

	if accessToken == "" {
		c.Redirect(http.StatusFound, "/forgot-password?error=session_expired")
		return
	}

	// Get form data
	newPassword := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if newPassword != confirmPassword {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{
			"title": "Reset Password",
			"error": "Passwords do not match",
		})
		return
	}

	if len(newPassword) < 6 {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{
			"title": "Reset Password",
			"error": "Password must be at least 6 characters",
		})
		return
	}

	// Update password in Supabase
	client := GetSupabaseClient()
	ctx := context.Background()

	// Update user password
	_, err := client.Auth.UpdateUser(ctx, accessToken, map[string]interface{}{
		"password": newPassword,
	})

	if err != nil {
		log.Printf("Error updating password: %v", err)
		log.Printf("Password update error details: %+v", err)
		renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{
			"title": "Reset Password",
			"error": fmt.Sprintf("Failed to reset password: %v", err),
		})
		return
	}

	log.Printf("Password reset successful")

	// Clear reset session cookie
	c.SetCookie("reset_access_token", "", -1, "/", "", false, true)

	// Show success page with login link
	renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{
		"title": "Reset Password",
		"success": true,
	})
}

// extractEmailFromRedirect attempts to extract email from redirect URL
func extractEmailFromRedirect(redirectTo string) string {
	// Parse the redirect URL to extract email if present
	if redirectTo == "" {
		return ""
	}

	// Try to parse as URL
	parsedURL, err := url.Parse(redirectTo)
	if err != nil {
		return ""
	}

	// Check if email is in query parameters
	queryParams := parsedURL.Query()
	if email := queryParams.Get("email"); email != "" {
		return email
	}

	return ""
}