package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UnifiedAuthMiddleware checks for valid authentication using either system
func UnifiedAuthMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authenticated := false
		var userID string
		var userEmail string
		var userRole string
		
		// Try Supabase Auth first if enabled
		if UseSupabaseAuth() {
			accessToken, err := c.Cookie("sb_access_token")
			if err == nil && accessToken != "" {
				client := GetSupabaseClient()
				ctx := context.Background()
				user, err := client.Auth.User(ctx, accessToken)
				
				if err == nil {
					authenticated = true
					userID = user.ID
					userEmail = user.Email
					userRole = "merchant"
					if user.UserMetadata != nil {
						if r, ok := user.UserMetadata["role"].(string); ok {
							userRole = r
						}
					}
				} else {
					// Try to refresh token
					refreshToken, _ := c.Cookie("sb_refresh_token")
					if refreshToken != "" {
						resp, err := client.Auth.RefreshUser(ctx, accessToken, refreshToken)
						if err == nil {
							// Update cookies
							c.SetCookie("sb_access_token", resp.AccessToken, 3600, "/", "", false, true)
							c.SetCookie("sb_refresh_token", resp.RefreshToken, 86400*7, "/", "", false, true)
							
							authenticated = true
							userID = resp.User.ID
							userEmail = resp.User.Email
							userRole = "merchant"
							if resp.User.UserMetadata != nil {
								if r, ok := resp.User.UserMetadata["role"].(string); ok {
									userRole = r
								}
							}
						}
					}
				}
			}
		}
		
		// Fall back to legacy JWT if not authenticated and legacy is enabled
		if !authenticated && UseLegacyAuth() {
			token, err := c.Cookie("auth_token")
			if err == nil && token != "" {
				claims, err := ValidateJWT(token)
				if err == nil {
					authenticated = true
					userID = string(claims.UserID)
					userEmail = claims.Email
					userRole = claims.Role
				}
			}
		}
		
		// Check if authenticated
		if !authenticated {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		
		// Check role if specified
		if requiredRole != "" && userRole != requiredRole {
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Access denied",
			})
			c.Abort()
			return
		}
		
		// Set user info in context
		c.Set("user_id", userID)
		c.Set("user_role", userRole)
		c.Set("user_email", userEmail)
		
		c.Next()
	}
}

// UnifiedRedirectIfAuthenticated redirects authenticated users from auth pages
func UnifiedRedirectIfAuthenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		authenticated := false
		var userRole string
		
		// Check Supabase Auth if enabled
		if UseSupabaseAuth() {
			accessToken, err := c.Cookie("sb_access_token")
			if err == nil && accessToken != "" {
				client := GetSupabaseClient()
				ctx := context.Background()
				user, err := client.Auth.User(ctx, accessToken)
				
				if err == nil {
					authenticated = true
					userRole = "merchant"
					if user.UserMetadata != nil {
						if r, ok := user.UserMetadata["role"].(string); ok {
							userRole = r
						}
					}
				}
			}
		}
		
		// Check legacy JWT if not authenticated and legacy is enabled
		if !authenticated && UseLegacyAuth() {
			token, err := c.Cookie("auth_token")
			if err == nil && token != "" {
				claims, err := ValidateJWT(token)
				if err == nil {
					authenticated = true
					userRole = claims.Role
				}
			}
		}
		
		if authenticated {
			if userRole == "admin" {
				c.Redirect(http.StatusFound, "/admin")
			} else {
				c.Redirect(http.StatusFound, "/dashboard")
			}
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// UnifiedLogin handles login with appropriate auth system
func UnifiedLogin(c *gin.Context) {
	// If Supabase Auth is enabled, try it first
	if UseSupabaseAuth() {
		// Try Supabase login
		SupabaseLogin(c)
		return
	}
	
	// Fall back to legacy login
	h := &Handlers{db: nil} // This needs proper initialization
	h.Login(c)
}

// UnifiedRegister handles registration with appropriate auth system
func UnifiedRegister(c *gin.Context) {
	// If Supabase Auth is enabled, use it
	if UseSupabaseAuth() {
		SupabaseRegister(c)
		return
	}
	
	// Fall back to legacy registration
	h := &Handlers{db: nil} // This needs proper initialization
	h.Register(c)
}

// UnifiedLogout handles logout for both auth systems
func UnifiedLogout(c *gin.Context) {
	// Clear Supabase tokens if present
	accessToken, _ := c.Cookie("sb_access_token")
	if accessToken != "" {
		client := GetSupabaseClient()
		ctx := context.Background()
		client.Auth.SignOut(ctx, accessToken)
	}
	
	// Clear all auth cookies
	c.SetCookie("sb_access_token", "", -1, "/", "", false, true)
	c.SetCookie("sb_refresh_token", "", -1, "/", "", false, true)
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	
	c.Redirect(http.StatusFound, "/")
}