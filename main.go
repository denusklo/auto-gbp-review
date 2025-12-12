package main

import (
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// renderPage renders a page with a specific layout
func renderPage(c *gin.Context, layout string, content string, data gin.H) {
	tmpl, err := template.ParseFiles(layout, content)
	if err != nil {
		log.Printf("Template parsing error: %v", err)
		c.String(http.StatusInternalServerError, "Template parsing error: %s", err.Error())
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
		c.String(http.StatusInternalServerError, "Template execution error: %s", err.Error())
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

	// Start the keep-alive pinger to prevent Render.com spin down
	go startKeepAlivePinger()

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// InitRoutes sets up all application routes
func InitRoutes(router *gin.Engine, db *Database) {
	// Create handlers
	handlers := NewHandlers(db)
	socialMediaHandlers := NewSocialMediaHandlers(db)

	// Public routes
	router.GET("/", handlers.Home)
	router.GET("/merchant", handlers.MerchantPage) // ?bn=businessname

	// Auth routes (redirect if already logged in)
	router.GET("/login", SupabaseRedirectIfAuthenticated(), handlers.LoginPage)
	router.POST("/login", SupabaseLogin)
	router.GET("/register", SupabaseRedirectIfAuthenticated(), handlers.RegisterPage)
	router.POST("/register", SupabaseRegister)
	router.POST("/logout", SupabaseLogout)

	// Email verification route
	router.GET("/verify-email", func(c *gin.Context) {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/verify_email.html", gin.H{
			"title": "Email Verification",
		})
	})

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
		admin.GET("/audit-logs", handlers.AdminAuditLogs)
	}

	// Merchant routes (protected)
	merchant := router.Group("/dashboard")
	merchant.Use(SupabaseAuthMiddleware("merchant"))
	{
		merchant.GET("/", handlers.MerchantDashboard)
		merchant.GET("/profile", handlers.MerchantProfile)
		merchant.POST("/profile", handlers.UpdateMerchantProfile) // Changed from PUT to POST

		// Social media integrations
		merchant.GET("/integrations", socialMediaHandlers.IntegrationsPage)
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// API routes for HTMX
	api := router.Group("/api")
	{
		// Admin-only API routes
		adminAPI := api.Group("")
		adminAPI.Use(SupabaseAuthMiddleware("admin"))
		{
			adminAPI.POST("/merchants/:id/toggle-status", handlers.ToggleMerchantStatus)
		}

		// Public API for reviews data
		api.GET("/reviews/data/:merchantId", handlers.GetReviewsData)
		api.GET("/reviews/modal/:merchantId/:platform", handlers.GetReviewModal)

		// Public API for analytics tracking
		api.GET("/track/view", handlers.TrackPageView)
		api.GET("/track/click", handlers.TrackLinkClick)

		// Review routes (protected)
		reviewsAPI := api.Group("/reviews")
		reviewsAPI.Use(SupabaseAuthMiddleware("merchant"))
		{
			reviewsAPI.POST("/add", handlers.AddReview)
			reviewsAPI.DELETE("/:id", handlers.DeleteReview)
		}

		// Social media API routes (protected)
		socialMedia := api.Group("/social-media")
		socialMedia.Use(SupabaseAuthMiddleware("merchant"))
		{
			// OAuth routes
			socialMedia.GET("/connect/:platform", socialMediaHandlers.ConnectPlatform)
			socialMedia.GET("/callback/:platform", socialMediaHandlers.OAuthCallback)

			// Connection management
			socialMedia.GET("/connections", socialMediaHandlers.GetConnections)
			socialMedia.DELETE("/connections/:id", socialMediaHandlers.DisconnectPlatform)

			// Sync operations
			socialMedia.POST("/connections/:id/sync", socialMediaHandlers.TriggerSync)
			socialMedia.GET("/connections/:id/logs", socialMediaHandlers.GetSyncLogs)

			// Synced reviews
			socialMedia.GET("/reviews", socialMediaHandlers.GetSyncedReviews)
		}

		// Admin social media routes
		adminSocialMedia := api.Group("/admin/social-media")
		adminSocialMedia.Use(SupabaseAuthMiddleware("admin"))
		{
			adminSocialMedia.GET("/connections", socialMediaHandlers.AdminConnectionsPage)
		}
	}
}

// startKeepAlivePinger starts a goroutine that pings the health endpoint every 14 minutes
// to prevent Render.com free tier from spinning down due to inactivity
func startKeepAlivePinger() {
	// Only run keep-alive in production (when deployed to Render.com)
	if os.Getenv("RENDER") != "true" {
		log.Println("Keep-alive pinger disabled (not running on Render.com)")
		return
	}

	// Get base URL from environment variable
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		log.Println("BASE_URL environment variable not set, skipping keep-alive pinger")
		return
	}

	// Parse base URL and add health path
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		log.Printf("Invalid BASE_URL: %v, skipping keep-alive pinger", err)
		return
	}
	parsedURL.Path = "/health"
	healthURL := parsedURL.String()

	// Ping every 5 seconds for testing (switch back to 14 minutes for production)
	interval := 14 * time.Minute // Production: 14 minutes
	// interval := 5 * time.Second // Testing: 5 seconds

	log.Printf("Starting keep-alive pinger - will ping %s every %s", healthURL, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go func() {
				client := &http.Client{
					Timeout: 30 * time.Second,
				}

				resp, err := client.Get(healthURL)
				if err != nil {
					log.Printf("Keep-alive ping failed: %v", err)
					return
				}
				defer resp.Body.Close()

				// Read and discard response body to avoid resource leaks
				if _, err := io.Copy(io.Discard, resp.Body); err != nil {
					log.Printf("Error discarding response body: %v", err)
				}

				log.Printf("Keep-alive ping successful: Status %d at %s",
					resp.StatusCode, time.Now().Format(time.RFC3339))
			}()
		}
	}
}

// Old JWT middleware - DEPRECATED, now using Supabase Auth middleware
// These functions have been removed - now using SupabaseAuthMiddleware and SupabaseRedirectIfAuthenticated
