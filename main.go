package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// renderPage renders a page with a specific layout
func renderPage(c *gin.Context, layout string, content string, data gin.H) {
	tmpl, err := template.ParseFiles(layout, content)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Template parsing error: " + err.Error(),
		})
		return
	}

	// Set default title if not provided
	if data == nil {
		data = gin.H{}
	}
	if _, exists := data["title"]; !exists {
		data["title"] = "ViralEngine"
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	err = tmpl.Execute(c.Writer, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Template execution error: " + err.Error(),
		})
	}
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize Supabase client
	if err := InitSupabase(); err != nil {
		log.Fatal("Failed to initialize Supabase client:", err)
	}

	// Initialize database connection
	db, err := InitDatabase()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Initialize Gin router
	router := gin.Default()

	// Serve static files
	router.Static("/static", "./static")

	// Initialize routes
	InitRoutes(router, db)

	// Get port from environment or default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// InitRoutes sets up all application routes
func InitRoutes(router *gin.Engine, db *Database) {
	// Create handlers
	handlers := NewHandlers(db)

	// Public routes
	router.GET("/", handlers.Home)
	router.GET("/merchant", handlers.MerchantPage) // ?bn=businessname

	// Auth routes (redirect if already logged in)
	router.GET("/login", SupabaseRedirectIfAuthenticated(), handlers.LoginPage)
	router.POST("/login", SupabaseLogin)
	router.GET("/register", SupabaseRedirectIfAuthenticated(), handlers.RegisterPage)
	router.POST("/register", SupabaseRegister)
	router.POST("/logout", SupabaseLogout)

	// Password reset routes (Supabase Auth only)
	router.GET("/forgot-password", SupabaseRedirectIfAuthenticated(), ForgotPasswordPage)
	router.POST("/forgot-password", ForgotPassword)
	router.GET("/reset-password", ResetPasswordPage)
	router.POST("/api/reset-password", ResetPassword)

	// Admin routes (protected)
	admin := router.Group("/admin")
	admin.Use(SupabaseAuthMiddleware("admin"))
	{
		admin.GET("/", handlers.AdminDashboard)
		admin.GET("/merchants", handlers.AdminMerchantsList)
		admin.GET("/merchants/new", handlers.AdminMerchantForm)
		admin.POST("/merchants", handlers.AdminCreateMerchant)
		admin.GET("/merchants/:id/edit", handlers.AdminEditMerchant)
		admin.POST("/merchants/:id/update", handlers.AdminUpdateMerchant) // Changed from PUT to POST
		admin.POST("/merchants/:id/delete", handlers.AdminDeleteMerchant) // Changed from DELETE to POST
	}

	// Merchant routes (protected)
	merchant := router.Group("/dashboard")
	merchant.Use(SupabaseAuthMiddleware("merchant"))
	{
		merchant.GET("/", handlers.MerchantDashboard)
		merchant.GET("/profile", handlers.MerchantProfile)
		merchant.POST("/profile", handlers.UpdateMerchantProfile) // Changed from PUT to POST
	}

	// API routes for HTMX
	api := router.Group("/api")
	{
		api.POST("/merchants/:id/toggle-status", handlers.ToggleMerchantStatus)
	}
}

// RedirectIfAuthenticated middleware redirects authenticated users to dashboard
func RedirectIfAuthenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from cookie
		token, err := c.Cookie("auth_token")
		if err != nil {
			// No token, continue to login/register page
			c.Next()
			return
		}

		// Validate token
		claims, err := ValidateJWT(token)
		if err != nil {
			// Invalid token, continue to login/register page
			c.Next()
			return
		}

		// Valid token found, redirect based on role
		if claims.Role == "admin" {
			c.Redirect(http.StatusFound, "/admin")
		} else {
			c.Redirect(http.StatusFound, "/dashboard")
		}
		c.Abort()
	}
}

// AuthMiddleware checks for valid authentication and role
func AuthMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from cookie
		token, err := c.Cookie("auth_token")
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Validate token and get user info
		claims, err := ValidateJWT(token)
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Check role if specified
		if requiredRole != "" && claims.Role != requiredRole {
			renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
				"error": "Access denied",
			})
			c.Abort()
			return
		}

		// Set user info in context
		c.Set("user_id", claims.UserID)
		c.Set("user_role", claims.Role)
		c.Set("user_email", claims.Email)

		c.Next()
	}
}
