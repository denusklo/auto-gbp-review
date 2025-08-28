package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
	supa "github.com/nedpals/supabase-go"
)

// SupabaseLogin handles user login with Supabase Auth
func SupabaseLogin(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")

	client := GetSupabaseClient()
	ctx := context.Background()
	
	// Sign in with email and password
	user, err := client.Auth.SignIn(ctx, supa.UserCredentials{
		Email:    email,
		Password: password,
	})
	
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/login.html", gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	// Set the access token as a cookie
	c.SetCookie("sb_access_token", user.AccessToken, 3600, "/", "", false, true)
	c.SetCookie("sb_refresh_token", user.RefreshToken, 86400*7, "/", "", false, true)
	
	// Get user role from metadata or database
	role := "merchant"
	if user.User.UserMetadata != nil {
		if r, ok := user.User.UserMetadata["role"].(string); ok {
			role = r
		}
	}
	
	// Redirect based on role
	if role == "admin" {
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
	ctx := context.Background()
	
	// Sign up with email and password
	_, err := client.Auth.SignUp(ctx, supa.UserCredentials{
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
	accessToken, _ := c.Cookie("sb_access_token")
	
	if accessToken != "" {
		client := GetSupabaseClient()
		ctx := context.Background()
		err := client.Auth.SignOut(ctx, accessToken)
		if err != nil {
			// Log error but continue with logout
			fmt.Printf("Logout error: %v\n", err)
		}
	}
	
	// Clear cookies
	c.SetCookie("sb_access_token", "", -1, "/", "", false, true)
	c.SetCookie("sb_refresh_token", "", -1, "/", "", false, true)
	c.SetCookie("auth_token", "", -1, "/", "", false, true) // Clear old JWT cookie too
	
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
		ctx := context.Background()
		user, err := client.Auth.User(ctx, accessToken)
		
		if err != nil {
			// Try to refresh the token
			refreshToken, _ := c.Cookie("sb_refresh_token")
			if refreshToken != "" {
				newUser, err := client.Auth.RefreshUser(ctx, accessToken, refreshToken)
				if err == nil {
					// Update cookies with new tokens
					c.SetCookie("sb_access_token", newUser.AccessToken, 3600, "/", "", false, true)
					c.SetCookie("sb_refresh_token", newUser.RefreshToken, 86400*7, "/", "", false, true)
					
					user = &newUser.User
				}
			}
			
			if err != nil {
				c.Redirect(http.StatusFound, "/login")
				c.Abort()
				return
			}
		}
		
		// Check role if specified
		role := "merchant"
		if user.UserMetadata != nil {
			if r, ok := user.UserMetadata["role"].(string); ok {
				role = r
			}
		}
		
		if requiredRole != "" && role != requiredRole {
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Access denied",
			})
			c.Abort()
			return
		}
		
		// Set user info in context
		c.Set("user_id", user.ID)
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
		ctx := context.Background()
		user, err := client.Auth.User(ctx, accessToken)
		
		if err != nil {
			// Invalid token, continue to login/register page
			c.Next()
			return
		}
		
		// Valid token found, redirect based on role
		role := "merchant"
		if user.UserMetadata != nil {
			if r, ok := user.UserMetadata["role"].(string); ok {
				role = r
			}
		}
		
		if role == "admin" {
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
	email := c.PostForm("email")
	log.Printf("Password reset requested for: %s", email)
	
	client := GetSupabaseClient()
	ctx := context.Background()
	
	// Check if user exists using Supabase Management API
	userExists, err := checkUserExistsSupabase(email)
	log.Printf("User check for %s: exists=%t, err=%v", email, userExists, err)
	
	if err != nil {
		log.Printf("Error checking user existence: %v", err)
		// Continue with password reset attempt for security
	} else if !userExists {
		log.Printf("User %s does not exist, showing error", email)
		renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{
			"error": "No account found with this email address.",
		})
		return
	}
	
	log.Printf("User %s exists, proceeding with password reset", email)
	
	// Request password reset - use environment-aware redirect URL
	redirectURL := getResetPasswordURL(c)
	log.Printf("Sending password reset for %s to redirect URL: %s", email, redirectURL)
	
	err = client.Auth.ResetPasswordForEmail(ctx, email, redirectURL)
	
	if err != nil {
		log.Printf("Password reset error for %s: %v", email, err)
		renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{
			"error": "Failed to send reset email. Please check your email address and try again.",
		})
		return
	}
	
	renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{
		"success": true,
	})
}

// checkUserExistsSupabase checks if a user exists using Node.js helper
func checkUserExistsSupabase(email string) (bool, error) {
	cmd := exec.Command("node", "check_user.js", email)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	
	result := strings.TrimSpace(string(output))
	return result == "true", nil
}

// getResetPasswordURL returns the appropriate reset password URL
func getResetPasswordURL(c *gin.Context) string {
	// Get the host from the request
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	host := c.Request.Host
	return fmt.Sprintf("%s://%s/reset-password", scheme, host)
}

// ResetPasswordPage renders the reset password form (when user clicks link in email)
func ResetPasswordPage(c *gin.Context) {
	// The token will be in the URL fragment, handled by JavaScript
	renderPage(c, "templates/layouts/auth.html", "templates/auth/reset_password.html", gin.H{
		"title": "Set New Password",
	})
}

// ResetPassword handles the password update
func ResetPassword(c *gin.Context) {
	accessToken := c.PostForm("access_token")
	newPassword := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")
	
	if newPassword != confirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Passwords do not match",
		})
		return
	}
	
	client := GetSupabaseClient()
	ctx := context.Background()
	
	// Update password using the access token from the reset link
	user, err := client.Auth.UpdateUser(ctx, accessToken, map[string]interface{}{
		"password": newPassword,
	})
	
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to reset password",
		})
		return
	}
	
	// User successfully updated
	_ = user
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}