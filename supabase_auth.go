package main

import (
	"context"
	"fmt"
	"net/http"
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
	user, err := client.Auth.SignUp(ctx, supa.UserCredentials{
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
	
	// Auto-login after registration
	loginUser, err := client.Auth.SignIn(ctx, supa.UserCredentials{
		Email:    email,
		Password: password,
	})
	
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": "Registration successful but login failed. Please login manually.",
		})
		return
	}
	
	// Set cookies
	c.SetCookie("sb_access_token", loginUser.AccessToken, 3600, "/", "", false, true)
	c.SetCookie("sb_refresh_token", loginUser.RefreshToken, 86400*7, "/", "", false, true)
	
	// For new registrations, return the user object for verification
	_ = user // Using the signup user data if needed
	
	c.Redirect(http.StatusFound, "/dashboard")
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
	
	client := GetSupabaseClient()
	ctx := context.Background()
	
	// Request password reset
	err := client.Auth.SendMagicLink(ctx, email)
	
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{
			"error": "Failed to send reset email. Please try again.",
		})
		return
	}
	
	renderPage(c, "templates/layouts/auth.html", "templates/auth/forgot_password.html", gin.H{
		"success": true,
	})
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