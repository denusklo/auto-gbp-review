package auth

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/supabase-community/gotrue-go/types"
)

// JWTClaims represents the custom claims in the Supabase JWT
type JWTClaims struct {
	UserRole string `json:"user_role"`
	jwt.RegisteredClaims
}

// hasRequiredRole checks if the user's role satisfies the required role
// Role hierarchy: superadmin > admin > merchant
func hasRequiredRole(userRole, requiredRole string) bool {
	// If no specific role required, allow all authenticated users
	if requiredRole == "" {
		return true
	}

	// Superadmin has access to everything
	if userRole == "superadmin" {
		return true
	}

	// Admin has access to admin and merchant routes
	if userRole == "admin" && (requiredRole == "admin" || requiredRole == "merchant") {
		return true
	}

	// Exact match
	return userRole == requiredRole
}

// extractRoleFromJWT decodes the JWT and extracts the user_role custom claim
func extractRoleFromJWT(tokenString string) (string, error) {
	// Parse the JWT without verification (Supabase already verified it)
	// We just need to extract the claims
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())

	token, _, err := parser.ParseUnverified(tokenString, &JWTClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse JWT: %w", err)
	}

	// Extract custom claims
	if claims, ok := token.Claims.(*JWTClaims); ok {
		if claims.UserRole != "" {
			return claims.UserRole, nil
		}
	}

	// Fallback: try parsing as generic claims to get user_role
	var claimsMap jwt.MapClaims
	parser2 := jwt.NewParser(jwt.WithoutClaimsValidation())
	token2, _, err := parser2.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "merchant", nil // Default to merchant if parsing fails
	}

	claimsMap = token2.Claims.(jwt.MapClaims)
	if userRole, ok := claimsMap["user_role"].(string); ok {
		return userRole, nil
	}

	// Default to merchant if no role found
	return "merchant", nil
}

// SupabaseLogin handles user login with Supabase Auth
func SupabaseLogin(c *gin.Context) {
	if !sameOrigin(c) {
		c.String(http.StatusForbidden, "Sign-in must be submitted from this site.")
		return
	}
	email := c.PostForm("email")
	password := c.PostForm("password")

	client := GetSupabaseClient()

	// Use Auth directly: the top-level sign-in method mutates shared session state.
	user, err := client.Auth.SignInWithEmailPassword(email, password)

	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/login.html", gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	// Set the access token as a cookie
	authCookie(c, "sb_access_token", user.AccessToken, 3600)
	authCookie(c, "sb_refresh_token", user.RefreshToken, 86400*7)

	// Get user role from JWT custom claims (injected by Auth Hook)
	role, err := extractRoleFromJWT(user.AccessToken)
	if err != nil {
		log.Printf("Error extracting role from JWT: %v", err)
		role = "merchant" // Default to merchant
	}

	// Redirect based on role
	if role == "admin" || role == "superadmin" {
		c.Redirect(http.StatusFound, "/admin")
	} else {
		c.Redirect(http.StatusFound, "/dashboard")
	}
}

// SupabaseRegister handles user registration with Supabase Auth
func SupabaseRegister(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if password != confirmPassword {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": "Passwords do not match",
		})
		return
	}

	client := GetSupabaseClient()

	// Sign up with email and password
	_, err := client.Auth.Signup(types.SignupRequest{
		Email:    email,
		Password: password,
		Data: map[string]interface{}{
			"role": "merchant",
		},
	})

	if err != nil {
		errorMsg := "Registration failed"
		if strings.Contains(err.Error(), "already registered") {
			errorMsg = "Email already exists"
		}
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": errorMsg,
		})
		return
	}

	// Registration successful - always show success message
	// Supabase will send confirmation email if required
	renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
		"success": "Registration successful! Please check your email to confirm your account, then return to login.",
	})
}

// SupabaseLogout handles user logout
func SupabaseLogout(c *gin.Context) {
	if !sameOrigin(c) {
		c.String(http.StatusForbidden, "Sign-out must be submitted from this site.")
		return
	}
	accessToken, _ := c.Cookie("sb_access_token")

	if accessToken != "" {
		client := GetSupabaseClient()
		err := client.Auth.WithToken(accessToken).Logout()
		if err != nil {
			// Log error but continue with logout
			log.Print("Supabase logout failed")
		}
	}

	// Clear cookies
	authCookie(c, "sb_access_token", "", -1)
	authCookie(c, "sb_refresh_token", "", -1)
	authCookie(c, "auth_token", "", -1) // Clear old JWT cookie too

	c.Redirect(http.StatusFound, "/")
}

// SupabaseAuthMiddleware validates Supabase Auth tokens
func SupabaseAuthMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get access token from cookie
		accessToken, err := c.Cookie("sb_access_token")
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Validate token with Supabase
		client := GetSupabaseClient()
		user, err := client.Auth.WithToken(accessToken).GetUser()

		if err != nil {
			// Try to refresh the token
			refreshToken, _ := c.Cookie("sb_refresh_token")
			if refreshToken != "" {
				var newUser *types.TokenResponse
				newUser, err = client.Auth.WithToken(accessToken).RefreshToken(refreshToken)
				if err == nil {
					// Update cookies with new tokens
					authCookie(c, "sb_access_token", newUser.AccessToken, 3600)
					authCookie(c, "sb_refresh_token", newUser.RefreshToken, 86400*7)

					accessToken = newUser.AccessToken
					user = &types.UserResponse{User: newUser.User}
				}
			}

			if err != nil {
				c.Redirect(http.StatusFound, "/login")
				c.Abort()
				return
			}
		}

		// Get role from JWT custom claims (injected by Auth Hook)
		// The Auth Hook also checks if user is banned
		role, err := extractRoleFromJWT(accessToken)
		if err != nil {
			log.Printf("Error extracting role from JWT: %v", err)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Check if user has required role
		if requiredRole != "" && !hasRequiredRole(role, requiredRole) {
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Access denied. You don't have permission to access this page.",
			})
			c.Abort()
			return
		}

		// Set user info in context
		c.Set("user_id", user.ID.String())
		c.Set("user_role", role)
		c.Set("user_email", user.Email)

		c.Next()
	}
}

// SupabaseRedirectIfAuthenticated redirects authenticated users
func SupabaseRedirectIfAuthenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get access token from cookie
		accessToken, err := c.Cookie("sb_access_token")
		if err != nil {
			// No token, continue to login/register page
			c.Next()
			return
		}

		// Validate token with Supabase
		client := GetSupabaseClient()
		_, err = client.Auth.WithToken(accessToken).GetUser()

		if err != nil {
			// Invalid token, continue to login/register page
			c.Next()
			return
		}

		// Valid token found, redirect based on role from JWT
		role, err := extractRoleFromJWT(accessToken)
		if err != nil {
			log.Printf("Error extracting role from JWT: %v", err)
			role = "merchant" // Default to merchant
		}

		if role == "admin" || role == "superadmin" {
			c.Redirect(http.StatusFound, "/admin")
		} else {
			c.Redirect(http.StatusFound, "/dashboard")
		}
		c.Abort()
	}
}

// ForgotPasswordPage renders the forgot password page
func ForgotPasswordPage(c *gin.Context) {
	renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{
		"title": "Reset Password",
	})
}

// ForgotPassword handles password reset requests
func ForgotPassword(c *gin.Context) {
	redirectURL := strings.TrimRight(os.Getenv("BASE_URL"), "/") + "/auth/callback"
	if !validBaseURL(os.Getenv("BASE_URL")) {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if err := recoverSupabasePassword(c.Request.Context(), c.PostForm("email"), redirectURL); err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{"error": "Failed to send reset email. Please try again."})
		return
	}
	c.Redirect(http.StatusFound, "/forgot-password?reset_sent=true")
}

// ResetPasswordPage renders the reset password form (when user clicks link in email)
func ResetPasswordPage(c *gin.Context) {
	if _, err := c.Cookie("reset_access_token"); err != nil {
		c.Redirect(http.StatusFound, "/forgot-password?error=session_expired")
		return
	}
	nonce, err := c.Cookie("reset_csrf")
	if err != nil {
		c.Status(http.StatusForbidden)
		return
	}
	renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{
		"title": "Set New Password",
		"csrf":  nonce,
	})
}

// ResetPassword is the JSON-compatible entry point to the same cookie-scoped flow.
func ResetPassword(c *gin.Context) { ResetPasswordCallback(c) }
