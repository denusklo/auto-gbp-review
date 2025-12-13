package main

import (
	"auto-gbp-review/utils"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	db *Database
}

func NewHandlers(db *Database) *Handlers {
	return &Handlers{db: db}
}

// Home page
func (h *Handlers) Home(c *gin.Context) {
	// Check if there's an id parameter for business page
	businessID := c.Query("id")

	if businessID != "" {
		h.BusinessPage(c, businessID)
		return
	}

	log.Println("Rendering home page")
	renderPage(c, "templates/layouts/base.html", "templates/home.html", gin.H{
		"title": "ViralEngine",
		"Year":  time.Now().Year(),
	})
	log.Println("Home page rendered")
}

// BusinessPage displays a business page with review cards
func (h *Handlers) BusinessPage(c *gin.Context, businessID string) {
	// Try to get merchant by ID first (if it's numeric)
	var merchant *Merchant
	var err error

	// Check if businessID is numeric (merchant ID) or slug
	if id, parseErr := strconv.Atoi(businessID); parseErr == nil {
		// It's a numeric ID
		merchant, err = h.getMerchantByID(id)
	} else {
		// It's a slug
		merchant, err = h.getMerchantBySlug(businessID)
	}

	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Business not found",
		})
		return
	}

	// Get merchant details
	details, err := h.getMerchantDetails(merchant.ID)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load business details",
		})
		return
	}

	// Get active reviews for this merchant
	reviews, err := h.getActiveReviewsByMerchantID(merchant.ID)
	if err != nil {
		log.Printf("Failed to fetch reviews for merchant %d: %v", merchant.ID, err)
		reviews = []Review{} // Empty slice if no reviews or error
	}

	// Clean phone number for tel: links
	cleanPhone := ""
	if details.PhoneNumber != "" {
		cleanPhone = strings.ReplaceAll(details.PhoneNumber, " ", "")
		cleanPhone = strings.ReplaceAll(cleanPhone, "(", "")
		cleanPhone = strings.ReplaceAll(cleanPhone, ")", "")
		cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")
		cleanPhone = strings.ReplaceAll(cleanPhone, ".", "")
	}

	googlePlaceID := ""
	if details.Address != "" {
		if placeID, err := utils.GetGooglePlaceID(merchant.BusinessName, details.Address); err == nil {
			googlePlaceID = placeID
		}
	}

	whatsappWebLink := ""
	whatsappAppLink := ""
	if details.PhoneNumber != "" && details.WhatsAppPresetText != "" {
		whatsappWebLink = utils.GenerateWhatsAppWebLink(cleanPhone, details.WhatsAppPresetText)
		whatsappAppLink = utils.GenerateWhatsAppAppLink(cleanPhone, details.WhatsAppPresetText)
	}

	wazeURL := ""
	if details.Address != "" {
		wazeURL = utils.GenerateWazeURL(merchant.BusinessName, details.Address, googlePlaceID)
	}

	renderPage(c, "templates/layouts/base.html", "templates/business.html", gin.H{
		"title":           merchant.BusinessName,
		"merchant":        merchant,
		"details":         details,
		"reviews":         reviews,
		"cleanPhone":      cleanPhone,
		"whatsappWebLink": whatsappWebLink,
		"whatsappAppLink": whatsappAppLink,
		"googlePlaceID":   googlePlaceID,
		"wazeURL":         wazeURL,
	})
}

// MerchantPage displays a merchant's page based on ?bn= parameter
func (h *Handlers) MerchantPage(c *gin.Context) {
	businessName := c.Query("bn")
	if businessName == "" {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Business name not specified",
		})
		return
	}

	// Get merchant data
	merchant, err := h.getMerchantBySlug(businessName)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Business not found",
		})
		return
	}

	// Get merchant details
	details, err := h.getMerchantDetails(merchant.ID)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load business details",
		})
		return
	}

	// Generate WhatsApp link
	whatsappWebLink := ""
	whatsappAppLink := ""
	if details.PhoneNumber != "" && details.WhatsAppPresetText != "" {
		whatsappWebLink = utils.GenerateWhatsAppWebLink(details.PhoneNumber, details.WhatsAppPresetText)
		whatsappAppLink = utils.GenerateWhatsAppAppLink(details.PhoneNumber, details.WhatsAppPresetText)
	}

	// Generate Google Review link
	googleReviewLink := ""
	if details.Address != "" {
		googleReviewLink = generateGoogleReviewLink(details.Address)
	}

	renderPage(c, "templates/layouts/base.html", "templates/merchant.html", gin.H{
		"merchant":           merchant,
		"details":            details,
		"whatsappWebLink":    whatsappWebLink, // Add this
		"whatsappAppLink":    whatsappAppLink, // Add this
		"google_review_link": googleReviewLink,
	})
}

// Auth handlers
func (h *Handlers) LoginPage(c *gin.Context) {
	renderPage(c, "templates/layouts/auth.html", "templates/auth/login.html", gin.H{
		"title": "Login",
	})
}

// Old JWT-based login - DEPRECATED, use SupabaseLogin instead
// func (h *Handlers) Login(c *gin.Context) {
//   This function has been removed - now using Supabase Auth
// }

func (h *Handlers) RegisterPage(c *gin.Context) {
	renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
		"title": "Register",
	})
}

// Old JWT-based register - DEPRECATED, use SupabaseRegister instead
// func (h *Handlers) Register(c *gin.Context) {
//   This function has been removed - now using Supabase Auth
// }

func (h *Handlers) Logout(c *gin.Context) {
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

// Admin handlers
func (h *Handlers) AdminDashboard(c *gin.Context) {
	// Get stats from database
	var totalMerchants, activeMerchants, totalUsers int

	// Count total merchants
	err := h.db.QueryRow("SELECT COUNT(*) FROM merchants").Scan(&totalMerchants)
	if err != nil {
		log.Printf("Error counting total merchants: %v", err)
		totalMerchants = 0
	}

	// Count active merchants (is_active = true)
	err = h.db.QueryRow("SELECT COUNT(*) FROM merchants WHERE is_active = true").Scan(&activeMerchants)
	if err != nil {
		log.Printf("Error counting active merchants: %v", err)
		activeMerchants = 0
	}

	// Count total users from auth.users
	err = h.db.QueryRow("SELECT COUNT(*) FROM auth.users WHERE deleted_at IS NULL").Scan(&totalUsers)
	if err != nil {
		log.Printf("Error counting total users: %v", err)
		totalUsers = 0
	}

	renderPage(c, "templates/layouts/base.html", "templates/admin/dashboard.html", gin.H{
		"title":           "Admin Dashboard",
		"totalMerchants":  totalMerchants,
		"activeMerchants": activeMerchants,
		"totalUsers":      totalUsers,
	})
}

func (h *Handlers) AdminMerchantsList(c *gin.Context) {
	merchants, err := h.getAllMerchantsWithDetails()
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load merchants",
		})
		return
	}

	renderPage(c, "templates/layouts/base.html", "templates/admin/merchants.html", gin.H{
		"title":     "Manage Merchants",
		"merchants": merchants,
	})
}

func (h *Handlers) AdminAuditLogs(c *gin.Context) {
	// Get filter parameters
	filterAction := c.Query("action")
	filterUserEmail := c.Query("user_email")
	filterTargetID := c.Query("target_id")

	// Build query with filters
	query := `
		SELECT id, user_id, user_email, action, target_type, target_id,
		       details, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if filterAction != "" {
		query += fmt.Sprintf(" AND action = $%d", argCount)
		args = append(args, filterAction)
		argCount++
	}

	if filterUserEmail != "" {
		query += fmt.Sprintf(" AND user_email ILIKE $%d", argCount)
		args = append(args, "%"+filterUserEmail+"%")
		argCount++
	}

	if filterTargetID != "" {
		query += fmt.Sprintf(" AND target_id = $%d", argCount)
		args = append(args, filterTargetID)
		argCount++
	}

	query += " ORDER BY created_at DESC LIMIT 100"

	// Execute query
	rows, err := h.db.Query(query, args...)
	if err != nil {
		log.Printf("Error fetching audit logs: %v", err)
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load audit logs",
		})
		return
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var detailsJSON []byte
		err := rows.Scan(&log.ID, &log.UserID, &log.UserEmail, &log.Action, &log.TargetType,
			&log.TargetID, &detailsJSON, &log.IPAddress, &log.UserAgent, &log.CreatedAt)
		if err != nil {
			log := log // Shadow to avoid confusion
			_ = log
			continue
		}
		// Format JSON for display
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, detailsJSON, "", "  "); err == nil {
			log.DetailsJSON = prettyJSON.String()
		} else {
			log.DetailsJSON = string(detailsJSON)
		}
		logs = append(logs, log)
	}

	// Get stats
	var totalLogs, createdCount, modifiedCount, last24hCount int

	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&totalLogs)
	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action LIKE '%_created'").Scan(&createdCount)
	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action LIKE '%_enabled' OR action LIKE '%_disabled' OR action LIKE '%_updated'").Scan(&modifiedCount)
	h.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE created_at > NOW() - INTERVAL '24 hours'").Scan(&last24hCount)

	renderPage(c, "templates/layouts/base.html", "templates/admin/audit_logs.html", gin.H{
		"title":           "Audit Logs",
		"logs":            logs,
		"totalLogs":       totalLogs,
		"createdCount":    createdCount,
		"modifiedCount":   modifiedCount,
		"last24hCount":    last24hCount,
		"filterAction":    filterAction,
		"filterUserEmail": filterUserEmail,
		"filterTargetID":  filterTargetID,
	})
}

func (h *Handlers) AdminMerchantForm(c *gin.Context) {
	renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_form.html", gin.H{
		"title": "Add New Merchant",
	})
}

func (h *Handlers) AdminCreateMerchant(c *gin.Context) {
	businessName := c.PostForm("business_name")
	slug := c.PostForm("slug")
	userEmail := c.PostForm("user_email")
	password := c.PostForm("password")

	// Check if user already exists
	existingUserID, err := h.getAuthUserByEmail(userEmail)

	var authUserID string

	if err == nil && existingUserID != "" {
		// User already exists - show error
		renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_form.html", gin.H{
			"title": "Add New Merchant",
			"error": "User with this email already exists. Please use a different email.",
		})
		return
	}

	// User doesn't exist - create new user AND role in one transaction
	authUserID, err = h.createSupabaseUserWithRole(userEmail, password, "merchant")
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_form.html", gin.H{
			"title": "Add New Merchant",
			"error": "Failed to create user account: " + err.Error(),
		})
		return
	}

	log.Printf("Successfully created user: %s with ID: %s", userEmail, authUserID)

	// Create merchant with auth_user_id
	merchantID, err := h.createMerchantWithAuthUserID(authUserID, businessName, slug)
	if err != nil {
		log.Printf("Failed to create merchant: %v", err)
		renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_form.html", gin.H{
			"title": "Add New Merchant",
			"error": "Failed to create merchant: " + err.Error(),
		})
		return
	}

	// Create default merchant details
	err = h.createMerchantDetails(merchantID)
	if err != nil {
		log.Printf("Failed to create merchant details: %v", err)
	}

	log.Printf("Successfully created merchant ID: %d for user: %s", merchantID, userEmail)

	// Log audit event
	h.logAuditEvent(c, "merchant_created", "merchant", fmt.Sprintf("%d", merchantID), map[string]interface{}{
		"business_name": businessName,
		"slug":          slug,
		"user_email":    userEmail,
		"auth_user_id":  authUserID,
	})

	c.Redirect(http.StatusFound, "/admin/merchants")
}

func (h *Handlers) AdminEditMerchant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Invalid merchant ID",
		})
		return
	}

	merchant, err := h.getMerchantByID(id)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Merchant not found",
		})
		return
	}

	details, err := h.getMerchantDetails(id)
	if err != nil {
		// Create default details if they don't exist
		details = &MerchantDetails{MerchantID: id}
	}

	renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_edit.html", gin.H{
		"title":    "Edit Merchant",
		"merchant": merchant,
		"details":  details,
	})
}

func (h *Handlers) AdminUpdateMerchant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Invalid merchant ID",
		})
		return
	}

	// Update merchant
	businessName := c.PostForm("business_name")
	slug := c.PostForm("slug")
	isActive := c.PostForm("is_active") == "true"

	err = h.updateMerchant(id, businessName, slug, isActive)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to update merchant: " + err.Error(),
		})
		return
	}

	// Update merchant details
	details := &MerchantDetails{
		MerchantID:         id,
		Address:            c.PostForm("address"),
		PhoneNumber:        c.PostForm("phone_number"),
		WhatsAppPresetText: c.PostForm("whatsapp_preset_text"),
		FacebookURL:        c.PostForm("facebook_url"),
		XiaohongshuID:      c.PostForm("xiaohongshu_id"),
		TiktokURL:          c.PostForm("tiktok_url"),
		InstagramURL:       c.PostForm("instagram_url"),
		ThreadsURL:         c.PostForm("threads_url"),
		WebsiteURL:         c.PostForm("website_url"),
		GooglePlayURL:      c.PostForm("google_play_url"),
		AppStoreURL:        c.PostForm("app_store_url"),
		GoogleMapsURL:      c.PostForm("google_maps_url"),
		WazeURL:            c.PostForm("waze_url"),
		LogoURL:            c.PostForm("logo_url"),
		ThemeColor:         c.PostForm("theme_color"),
	}

	err = h.updateMerchantDetails(details)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to update merchant details: " + err.Error(),
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/merchants")
}

func (h *Handlers) AdminDeleteMerchant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Invalid merchant ID",
		})
		return
	}

	err = h.deleteMerchant(id)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to delete merchant",
		})
		return
	}

	c.Redirect(http.StatusFound, "/admin/merchants")
}

// Merchant handlers
func (h *Handlers) MerchantDashboard(c *gin.Context) {
	userID := c.GetString("user_id")
	log.Printf("Dashboard: Looking for merchants with auth_user_id: %s", userID)
	merchants, err := h.getMerchantsByAuthUserID(userID)
	log.Printf("Dashboard: Found %d merchants, error: %v", len(merchants), err)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load your businesses",
		})
		return
	}

	// Get analytics stats for first merchant (most users have one business)
	var stats map[string]interface{}
	if len(merchants) > 0 {
		merchantID := merchants[0].ID
		stats = h.getMerchantStats(merchantID)
	} else {
		stats = map[string]interface{}{
			"total_views":       0,
			"total_clicks":      0,
			"reviews_count":     0,
			"views_last_7days":  []int{},
			"clicks_by_platform": map[string]int{},
		}
	}

	renderPage(c, "templates/layouts/base.html", "templates/merchant_dashboard.html", gin.H{
		"title":     "Dashboard",
		"merchants": merchants,
		"stats":     stats,
	})
}

// getMerchantStats fetches analytics statistics for a merchant
func (h *Handlers) getMerchantStats(merchantID int) map[string]interface{} {
	stats := make(map[string]interface{})

	// Total page views
	var totalViews int
	h.db.QueryRow("SELECT COUNT(*) FROM page_views WHERE merchant_id = $1", merchantID).Scan(&totalViews)
	stats["total_views"] = totalViews

	// Total link clicks
	var totalClicks int
	h.db.QueryRow("SELECT COUNT(*) FROM link_clicks WHERE merchant_id = $1", merchantID).Scan(&totalClicks)
	stats["total_clicks"] = totalClicks

	// Active reviews count
	var reviewsCount int
	h.db.QueryRow("SELECT COUNT(*) FROM merchant_reviews WHERE merchant_id = $1 AND is_active = true", merchantID).Scan(&reviewsCount)
	stats["reviews_count"] = reviewsCount

	// Views in last 7 days (for chart)
	rows, err := h.db.Query(`
		SELECT DATE(created_at) as date, COUNT(*) as count
		FROM page_views
		WHERE merchant_id = $1 AND created_at > NOW() - INTERVAL '7 days'
		GROUP BY DATE(created_at)
		ORDER BY date
	`, merchantID)
	if err == nil {
		defer rows.Close()
		viewsLast7Days := make([]map[string]interface{}, 0)
		for rows.Next() {
			var date time.Time
			var count int
			if err := rows.Scan(&date, &count); err == nil {
				viewsLast7Days = append(viewsLast7Days, map[string]interface{}{
					"date":  date.Format("Jan 2"),
					"count": count,
				})
			}
		}
		stats["views_last_7days"] = viewsLast7Days
	}

	// Clicks by platform (for pie chart)
	clicksRows, err := h.db.Query(`
		SELECT platform, COUNT(*) as count
		FROM link_clicks
		WHERE merchant_id = $1
		GROUP BY platform
		ORDER BY count DESC
	`, merchantID)
	if err == nil {
		defer clicksRows.Close()
		clicksByPlatform := make(map[string]int)
		for clicksRows.Next() {
			var platform string
			var count int
			if err := clicksRows.Scan(&platform, &count); err == nil {
				clicksByPlatform[platform] = count
			}
		}
		stats["clicks_by_platform"] = clicksByPlatform
	}

	// Unique visitors (based on distinct IP addresses)
	var uniqueVisitors int
	h.db.QueryRow("SELECT COUNT(DISTINCT ip_address) FROM page_views WHERE merchant_id = $1", merchantID).Scan(&uniqueVisitors)
	stats["unique_visitors"] = uniqueVisitors

	return stats
}

func (h *Handlers) MerchantProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	userEmail := c.GetString("user_email")
	merchants, err := h.getMerchantsByAuthUserID(userID)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load your businesses",
		})
		return
	}

	var merchant *Merchant
	var details *MerchantDetails

	if len(merchants) > 0 {
		merchant = &merchants[0]
		details, _ = h.getMerchantDetails(merchant.ID)
	}

	var reviews []Review
	if merchant != nil {
		reviews, _ = h.getReviewsByMerchantID(merchant.ID)
	}

	renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
		"title":     "Profile",
		"merchant":  merchant,
		"details":   details,
		"reviews":   reviews,
		"userEmail": userEmail,
	})
}

// Replace your existing UpdateMerchantProfile function in handlers.go with this:
func (h *Handlers) UpdateMerchantProfile(c *gin.Context) {
	userID := c.GetString("user_id")

	// Validate required fields
	var errors []string
	businessName := strings.TrimSpace(c.PostForm("business_name"))
	slug := strings.TrimSpace(c.PostForm("slug"))

	if businessName == "" {
		errors = append(errors, "Business Name is required")
	}
	if slug == "" {
		errors = append(errors, "URL Slug is required")
	}

	// If there are validation errors, return them
	if len(errors) > 0 {
		// Check if this is an AJAX request
		if c.GetHeader("HX-Request") != "" {
			// Return HTML with JavaScript to show error toasts
			var errorJS string
			for _, error := range errors {
				errorJS += fmt.Sprintf(`
					iziToast.error({
						title: 'Validation Error',
						message: '%s',
						icon: 'fas fa-exclamation-circle',
						timeout: 7000,
					});`, error)
			}
			html := fmt.Sprintf("<script>%s</script>", errorJS)
			c.Header("Content-Type", "text/html")
			c.String(http.StatusBadRequest, html)
			return
		}

		// For non-AJAX requests, get existing data and render page with errors
		merchants, _ := h.getMerchantsByAuthUserID(userID)
		var merchant *Merchant
		var details *MerchantDetails
		if len(merchants) > 0 {
			merchant = &merchants[0]
			details, _ = h.getMerchantDetails(merchant.ID)
		}

		errorMsg := strings.Join(errors, ", ")
		renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
			"title":     "Profile",
			"merchant":  merchant,
			"details":   details,
			"error":     errorMsg,
			"userEmail": c.GetString("user_email"),
		})
		return
	}

	// Get or create merchant (your existing logic)
	merchants, err := h.getMerchantsByAuthUserID(userID)
	if err != nil {
		if c.GetHeader("HX-Request") != "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"errors":  []string{"Failed to load your business"},
			})
			return
		}
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load your business",
		})
		return
	}

	var merchantID int
	var currentDetails *MerchantDetails

	if len(merchants) == 0 {
		// Create new merchant
		merchantID, err = h.createMerchantWithAuthUserID(userID, businessName, slug)
		if err != nil {
			if c.GetHeader("HX-Request") != "" {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"errors":  []string{"Failed to create business: " + err.Error()},
				})
				return
			}
			renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
				"title": "Profile",
				"error": "Failed to create business: " + err.Error(),
			})
			return
		}
		err = h.createMerchantDetails(merchantID)
		if err != nil {
			log.Printf("Failed to create merchant details: %v", err)
		}
	} else {
		merchantID = merchants[0].ID
		// Get current details to preserve existing logo if no new one uploaded
		currentDetails, _ = h.getMerchantDetails(merchantID)

		// Update existing merchant
		err = h.updateMerchant(merchantID, businessName, slug, true)
		if err != nil {
			if c.GetHeader("HX-Request") != "" {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"errors":  []string{"Failed to update business: " + err.Error()},
				})
				return
			}
			renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
				"title": "Profile",
				"error": "Failed to update business: " + err.Error(),
			})
			return
		}
	}

	// Handle logo upload or URL
	var logoURL string

	// Check if a file was uploaded
	file, header, err := c.Request.FormFile("logo_file")
	if err == nil && header.Size > 0 {
		// File was uploaded
		defer file.Close()

		// Validate file type
		contentType := header.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			if c.GetHeader("HX-Request") != "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"errors":  []string{"Please upload an image file (jpg, png, gif, webp)"},
				})
				return
			}
			// Get existing data for redisplay
			merchants, _ := h.getMerchantsByAuthUserID(userID)
			var merchant *Merchant
			var details *MerchantDetails
			if len(merchants) > 0 {
				merchant = &merchants[0]
				details, _ = h.getMerchantDetails(merchant.ID)
			}

			renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
				"title":    "Profile",
				"merchant": merchant,
				"details":  details,
				"error":    "Please upload an image file (jpg, png, gif, webp)",
			})
			return
		}

		// Upload to Supabase using the function from storage.go
		logoURL, err = uploadToSupabase(file, header, "logos")
		if err != nil {
			// Get existing data for redisplay
			merchants, _ := h.getMerchantsByAuthUserID(userID)
			var merchant *Merchant
			var details *MerchantDetails
			if len(merchants) > 0 {
				merchant = &merchants[0]
				details, _ = h.getMerchantDetails(merchant.ID)
			}

			renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
				"title":    "Profile",
				"merchant": merchant,
				"details":  details,
				"error":    "Failed to upload logo: " + err.Error(),
			})
			return
		}
	} else {
		// No file uploaded, check URL field
		urlFromForm := strings.TrimSpace(c.PostForm("logo_url"))
		if urlFromForm != "" {
			logoURL = urlFromForm
		} else if currentDetails != nil {
			// Keep existing logo if no new file or URL provided
			logoURL = currentDetails.LogoURL
		}
	}

	// Update merchant details (your existing logic)
	details := &MerchantDetails{
		MerchantID:         merchantID,
		Address:            c.PostForm("address"),
		PhoneNumber:        c.PostForm("phone_number"),
		WhatsAppPresetText: c.PostForm("whatsapp_preset_text"),
		FacebookURL:        c.PostForm("facebook_url"),
		XiaohongshuID:      c.PostForm("xiaohongshu_id"),
		TiktokURL:          c.PostForm("tiktok_url"),
		InstagramURL:       c.PostForm("instagram_url"),
		ThreadsURL:         c.PostForm("threads_url"),
		WebsiteURL:         c.PostForm("website_url"),
		GooglePlayURL:      c.PostForm("google_play_url"),
		AppStoreURL:        c.PostForm("app_store_url"),
		GoogleMapsURL:      c.PostForm("google_maps_url"),
		WazeURL:            c.PostForm("waze_url"),
		LogoURL:            logoURL, // This will be either uploaded URL or form URL or existing URL
		ThemeColor:         c.PostForm("theme_color"),
	}

	err = h.updateMerchantDetails(details)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
			"title": "Profile",
			"error": "Failed to update profile: " + err.Error(),
		})
		return
	}

	// Handle review updates if present
	reviewUpdatesJSON := c.PostForm("review_updates")
	if reviewUpdatesJSON != "" {
		var reviewUpdates []map[string]interface{}
		if err := json.Unmarshal([]byte(reviewUpdatesJSON), &reviewUpdates); err == nil {
			for _, update := range reviewUpdates {
				if reviewIDStr, ok := update["id"].(string); ok {
					if reviewID, err := strconv.Atoi(reviewIDStr); err == nil {
						platform := update["platform"].(string)
						text := update["text"].(string)
						isActive := update["is_active"].(bool)

						h.updateReview(reviewID, platform, text, isActive)
					}
				}
			}
		}
	}

	// Check if this is an AJAX request
	if c.GetHeader("HX-Request") != "" {
		// Return HTML with JavaScript to show toast
		html := `<script>
			iziToast.success({
				title: 'Profile Updated!',
				message: 'Your business profile has been successfully saved.',
				icon: 'fas fa-save',
			});
		</script>`
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, html)
		return
	}

	c.Redirect(http.StatusFound, "/dashboard/profile?success=1")
}

func (h *Handlers) ToggleMerchantStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid merchant ID"})
		return
	}

	// Get merchant details before toggling for audit log
	merchant, err := h.getMerchantByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Merchant not found"})
		return
	}
	oldStatus := merchant.IsActive

	// Toggle status
	err = h.toggleMerchantStatus(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle status"})
		return
	}

	// Get new status
	newStatus := !oldStatus

	// Log audit event
	action := "merchant_disabled"
	if newStatus {
		action = "merchant_enabled"
	}
	h.logAuditEvent(c, action, "merchant", idStr, map[string]interface{}{
		"business_name": merchant.BusinessName,
		"old_status":    oldStatus,
		"new_status":    newStatus,
	})

	c.JSON(http.StatusOK, gin.H{"status": "toggled"})
}

func generateGoogleReviewLink(address string) string {
	encodedAddress := url.QueryEscape(address)
	return fmt.Sprintf("https://www.google.com/maps/search/%s", encodedAddress)
}

// Database helper methods and structs
type User struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type Merchant struct {
	ID           int       `json:"id"`
	AuthUserID   string    `json:"auth_user_id"` // UUID from auth.users
	BusinessName string    `json:"business_name"`
	Slug         string    `json:"slug"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UserEmail    string    `json:"user_email,omitempty"` // For admin views (joined from auth.users)
}

type MerchantDetails struct {
	ID                 int    `json:"id"`
	MerchantID         int    `json:"merchant_id"`
	Address            string `json:"address"`
	PhoneNumber        string `json:"phone_number"`
	WhatsAppPresetText string `json:"whatsapp_preset_text"`
	FacebookURL        string `json:"facebook_url"`
	XiaohongshuID      string `json:"xiaohongshu_id"`
	TiktokURL          string `json:"tiktok_url"`
	InstagramURL       string `json:"instagram_url"`
	ThreadsURL         string `json:"threads_url"`
	WebsiteURL         string `json:"website_url"`
	GooglePlayURL      string `json:"google_play_url"`
	AppStoreURL        string `json:"app_store_url"`
	GoogleMapsURL      string `json:"google_maps_url"`
	WazeURL            string `json:"waze_url"`
	LogoURL            string `json:"logo_url"`
	ThemeColor         string `json:"theme_color"`
}

type Review struct {
	ID         int       `json:"id"`
	MerchantID int       `json:"merchant_id"`
	Platform   string    `json:"platform"`
	ReviewText string    `json:"review_text"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type AuditLog struct {
	ID          int64     `json:"id"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	Details     string    `json:"details"`      // JSON string
	DetailsJSON string    `json:"details_json"` // Formatted for display
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
}

// Database operations for merchants
func (h *Handlers) createMerchant(userID int, businessName, slug string) (int, error) {
	var merchantID int
	err := h.db.QueryRow("INSERT INTO merchants (user_id, business_name, slug) VALUES ($1, $2, $3) RETURNING id",
		userID, businessName, slug).Scan(&merchantID)
	return merchantID, err
}

func (h *Handlers) getMerchantByID(id int) (*Merchant, error) {
	merchant := &Merchant{}
	err := h.db.QueryRow("SELECT id, auth_user_id, business_name, slug, is_active, created_at FROM merchants WHERE id = $1", id).
		Scan(&merchant.ID, &merchant.AuthUserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt)
	return merchant, err
}

func (h *Handlers) updateMerchant(id int, businessName, slug string, isActive bool) error {
	_, err := h.db.Exec("UPDATE merchants SET business_name = $1, slug = $2, is_active = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $4",
		businessName, slug, isActive, id)
	return err
}

func (h *Handlers) deleteMerchant(id int) error {
	_, err := h.db.Exec("DELETE FROM merchants WHERE id = $1", id)
	return err
}

func (h *Handlers) toggleMerchantStatus(id int) error {
	_, err := h.db.Exec("UPDATE merchants SET is_active = NOT is_active, updated_at = CURRENT_TIMESTAMP WHERE id = $1", id)
	return err
}

// Database operations for merchant details
func (h *Handlers) createMerchantDetails(merchantID int) error {
	_, err := h.db.Exec("INSERT INTO merchant_details (merchant_id) VALUES ($1)", merchantID)
	return err
}

func (h *Handlers) updateMerchantDetails(details *MerchantDetails) error {
	_, err := h.db.Exec(`UPDATE merchant_details SET 
		address = $1, phone_number = $2, whatsapp_preset_text = $3, facebook_url = $4, 
		xiaohongshu_id = $5, tiktok_url = $6, instagram_url = $7, threads_url = $8,
		website_url = $9, google_play_url = $10, app_store_url = $11, google_maps_url = $12,
		waze_url = $13, logo_url = $14, theme_color = $15, updated_at = CURRENT_TIMESTAMP
		WHERE merchant_id = $16`,
		details.Address, details.PhoneNumber, details.WhatsAppPresetText, details.FacebookURL,
		details.XiaohongshuID, details.TiktokURL, details.InstagramURL, details.ThreadsURL,
		details.WebsiteURL, details.GooglePlayURL, details.AppStoreURL, details.GoogleMapsURL,
		details.WazeURL, details.LogoURL, details.ThemeColor, details.MerchantID)
	return err
}

// Existing database helper methods
func (h *Handlers) getUserByEmail(email string) (*User, error) {
	user := &User{}
	err := h.db.QueryRow("SELECT id, email, password_hash, role, created_at FROM users WHERE email = $1", email).
		Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt)
	return user, err
}

// getAuthUserByEmail gets user from auth.users table
func (h *Handlers) getAuthUserByEmail(email string) (string, error) {
	var userID string
	err := h.db.QueryRow("SELECT id FROM auth.users WHERE email = $1", email).Scan(&userID)
	return userID, err
}

// createSupabaseUserWithRole creates a new user via Supabase Admin API and sets their role
func (h *Handlers) createSupabaseUserWithRole(email, password, role string) (string, error) {
	supabaseURL := GetSupabaseURL()
	serviceRoleKey := GetSupabaseServiceKey()

	log.Printf("Creating Supabase user for email: %s with role: %s", email, role)
	log.Printf("Supabase URL: %s", supabaseURL)

	// Prepare request body - don't set user_metadata to avoid trigger conflict
	requestBody := map[string]interface{}{
		"email":         email,
		"password":      password,
		"email_confirm": true, // Auto-confirm email
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("Request body: %s", string(jsonData))

	// Make HTTP request to Supabase Admin API
	url := fmt.Sprintf("%s/auth/v1/admin/users", supabaseURL)
	log.Printf("Making request to: %s", url)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Failed to decode response body, status: %d", resp.StatusCode)
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Response status: %d", resp.StatusCode)
	log.Printf("Response body: %+v", result)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errorMsg := "Unknown error"
		if msg, ok := result["message"].(string); ok {
			errorMsg = msg
		} else if msg, ok := result["error"].(string); ok {
			errorMsg = msg
		} else if msg, ok := result["msg"].(string); ok {
			errorMsg = msg
		}
		log.Printf("API error - Status: %d, Message: %s, Full response: %+v", resp.StatusCode, errorMsg, result)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, errorMsg)
	}

	// Extract user ID from response
	userID, ok := result["id"].(string)
	if !ok {
		log.Printf("User ID not found in response: %+v", result)
		return "", fmt.Errorf("user ID not found in response")
	}

	log.Printf("Successfully created user with ID: %s, now creating role entry", userID)

	// Manually insert into user_roles table (bypassing trigger)
	_, err = h.db.Exec(`
		INSERT INTO public.user_roles (user_id, role)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET role = $2
	`, userID, role)

	if err != nil {
		log.Printf("Failed to create user_roles entry: %v", err)
		return "", fmt.Errorf("user created but failed to set role: %w", err)
	}

	log.Printf("Successfully created user_roles entry for user: %s", userID)
	return userID, nil
}

// createSupabaseUser creates a new user via Supabase Admin API
func (h *Handlers) createSupabaseUser(email, password string) (string, error) {
	supabaseURL := GetSupabaseURL()
	serviceRoleKey := GetSupabaseServiceKey()

	log.Printf("Creating Supabase user for email: %s", email)
	log.Printf("Supabase URL: %s", supabaseURL)

	// Prepare request body
	requestBody := map[string]interface{}{
		"email":         email,
		"password":      password,
		"email_confirm": true, // Auto-confirm email
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("Request body: %s", string(jsonData))

	// Make HTTP request to Supabase Admin API
	url := fmt.Sprintf("%s/auth/v1/admin/users", supabaseURL)
	log.Printf("Making request to: %s", url)

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("apikey", serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Failed to decode response body, status: %d", resp.StatusCode)
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Response status: %d", resp.StatusCode)
	log.Printf("Response body: %+v", result)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errorMsg := "Unknown error"
		if msg, ok := result["message"].(string); ok {
			errorMsg = msg
		} else if msg, ok := result["error"].(string); ok {
			errorMsg = msg
		} else if msg, ok := result["msg"].(string); ok {
			errorMsg = msg
		}
		log.Printf("API error - Status: %d, Message: %s, Full response: %+v", resp.StatusCode, errorMsg, result)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, errorMsg)
	}

	// Extract user ID from response
	userID, ok := result["id"].(string)
	if !ok {
		log.Printf("User ID not found in response: %+v", result)
		return "", fmt.Errorf("user ID not found in response")
	}

	log.Printf("Successfully created user with ID: %s", userID)
	return userID, nil
}

func (h *Handlers) createUser(email, passwordHash, role string) (int, error) {
	var userID int
	err := h.db.QueryRow("INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3) RETURNING id",
		email, passwordHash, role).Scan(&userID)
	return userID, err
}

func (h *Handlers) getMerchantBySlug(slug string) (*Merchant, error) {
	merchant := &Merchant{}
	err := h.db.QueryRow("SELECT id, auth_user_id, business_name, slug, is_active, created_at FROM merchants WHERE slug = $1 AND is_active = true", slug).
		Scan(&merchant.ID, &merchant.AuthUserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt)
	return merchant, err
}

func (h *Handlers) getMerchantDetails(merchantID int) (*MerchantDetails, error) {
	details := &MerchantDetails{}
	err := h.db.QueryRow(`SELECT id, merchant_id, COALESCE(address, ''), COALESCE(phone_number, ''), 
		COALESCE(whatsapp_preset_text, ''), COALESCE(facebook_url, ''), COALESCE(xiaohongshu_id, ''),
		COALESCE(tiktok_url, ''), COALESCE(instagram_url, ''), COALESCE(threads_url, ''),
		COALESCE(website_url, ''), COALESCE(google_play_url, ''), COALESCE(app_store_url, ''),
		COALESCE(google_maps_url, ''), COALESCE(waze_url, ''), COALESCE(logo_url, ''), 
		COALESCE(theme_color, '#3B82F6')
		FROM merchant_details WHERE merchant_id = $1`, merchantID).
		Scan(&details.ID, &details.MerchantID, &details.Address, &details.PhoneNumber,
			&details.WhatsAppPresetText, &details.FacebookURL, &details.XiaohongshuID,
			&details.TiktokURL, &details.InstagramURL, &details.ThreadsURL,
			&details.WebsiteURL, &details.GooglePlayURL, &details.AppStoreURL,
			&details.GoogleMapsURL, &details.WazeURL, &details.LogoURL, &details.ThemeColor)

	if err == sql.ErrNoRows {
		// Create default details if none exist
		err = h.createMerchantDetails(merchantID)
		if err != nil {
			return nil, err
		}
		return h.getMerchantDetails(merchantID)
	}

	return details, err
}

func (h *Handlers) getAllMerchants() ([]Merchant, error) {
	rows, err := h.db.Query("SELECT id, auth_user_id, business_name, slug, is_active, created_at FROM merchants ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merchants []Merchant
	for rows.Next() {
		var merchant Merchant
		if err := rows.Scan(&merchant.ID, &merchant.AuthUserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt); err != nil {
			return nil, err
		}
		merchants = append(merchants, merchant)
	}
	return merchants, nil
}

func (h *Handlers) getAllMerchantsWithDetails() ([]Merchant, error) {
	rows, err := h.db.Query(`
		SELECT m.id, m.auth_user_id, m.business_name, m.slug, m.is_active, m.created_at, u.email
		FROM merchants m
		LEFT JOIN auth.users u ON m.auth_user_id = u.id
		ORDER BY m.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merchants []Merchant
	for rows.Next() {
		var merchant Merchant
		if err := rows.Scan(&merchant.ID, &merchant.AuthUserID, &merchant.BusinessName, &merchant.Slug,
			&merchant.IsActive, &merchant.CreatedAt, &merchant.UserEmail); err != nil {
			return nil, err
		}
		merchants = append(merchants, merchant)
	}
	return merchants, nil
}

func (h *Handlers) getMerchantsByUserID(userID int) ([]Merchant, error) {
	// This function is deprecated - user_id column no longer exists
	// Return empty slice to prevent errors
	return []Merchant{}, nil
}

// Auth.users UUID-based functions (migrated from auth_user_helpers.go)
func (h *Handlers) createMerchantWithAuthUserID(authUserID, businessName, slug string) (int, error) {
	var merchantID int
	err := h.db.QueryRow("INSERT INTO merchants (auth_user_id, business_name, slug) VALUES ($1, $2, $3) RETURNING id",
		authUserID, businessName, slug).Scan(&merchantID)
	return merchantID, err
}

func (h *Handlers) getMerchantsByAuthUserID(authUserID string) ([]Merchant, error) {
	log.Printf("getMerchantsByAuthUserID: Querying for auth_user_id = %s", authUserID)
	rows, err := h.db.Query("SELECT id, auth_user_id, business_name, slug, is_active, created_at FROM merchants WHERE auth_user_id = $1 ORDER BY created_at DESC", authUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merchants []Merchant
	for rows.Next() {
		var merchant Merchant
		if err := rows.Scan(&merchant.ID, &merchant.AuthUserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt); err != nil {
			return nil, err
		}
		merchants = append(merchants, merchant)
	}
	return merchants, nil
}

// Review database operations
func (h *Handlers) getReviewsByMerchantID(merchantID int) ([]Review, error) {
	rows, err := h.db.Query(`
		SELECT id, merchant_id, platform, review_text, is_active, created_at, updated_at
		FROM merchant_reviews
		WHERE merchant_id = $1
		ORDER BY created_at DESC
	`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var review Review
		if err := rows.Scan(&review.ID, &review.MerchantID, &review.Platform,
			&review.ReviewText, &review.IsActive, &review.CreatedAt, &review.UpdatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func (h *Handlers) getActiveReviewsByMerchantID(merchantID int) ([]Review, error) {
	rows, err := h.db.Query(`
		SELECT id, merchant_id, platform, review_text, is_active, created_at, updated_at
		FROM merchant_reviews
		WHERE merchant_id = $1 AND is_active = true
		ORDER BY created_at DESC
	`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var review Review
		if err := rows.Scan(&review.ID, &review.MerchantID, &review.Platform,
			&review.ReviewText, &review.IsActive, &review.CreatedAt, &review.UpdatedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func (h *Handlers) createReview(merchantID int, platform, reviewText string) error {
	log.Printf("createReview: Inserting merchantID=%d, platform=%s, reviewText=%s", merchantID, platform, reviewText)
	_, err := h.db.Exec(`
		INSERT INTO merchant_reviews (merchant_id, platform, review_text, is_active)
		VALUES ($1, $2, $3, true)
	`, merchantID, platform, reviewText)
	if err != nil {
		log.Printf("createReview SQL error: %v", err)
	}
	return err
}

func (h *Handlers) updateReview(reviewID int, platform, reviewText string, isActive bool) error {
	_, err := h.db.Exec(`
		UPDATE merchant_reviews
		SET platform = $2, review_text = $3, is_active = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, reviewID, platform, reviewText, isActive)
	return err
}

func (h *Handlers) deleteReview(reviewID int) error {
	_, err := h.db.Exec("DELETE FROM merchant_reviews WHERE id = $1", reviewID)
	return err
}

// API handlers for reviews
func (h *Handlers) AddReview(c *gin.Context) {
	userID := c.GetString("user_id")
	log.Printf("AddReview: userID = %s", userID)

	// Get merchant for this user
	merchants, err := h.getMerchantsByAuthUserID(userID)
	if err != nil || len(merchants) == 0 {
		log.Printf("AddReview error: No merchant found for user %s, err: %v", userID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No merchant found"})
		return
	}

	merchantID := merchants[0].ID
	platform := c.PostForm("platform")
	reviewText := c.PostForm("text")

	log.Printf("AddReview: merchantID=%d, platform=%s, reviewText=%s", merchantID, platform, reviewText)

	if platform == "" || reviewText == "" {
		log.Printf("AddReview error: Missing fields - platform=%s, reviewText=%s", platform, reviewText)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Platform and text are required"})
		return
	}

	// Create review template with just platform and text
	err = h.createReview(merchantID, platform, reviewText)
	if err != nil {
		log.Printf("AddReview error: Failed to create review - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create review"})
		return
	}

	log.Printf("AddReview: Successfully created review template")

	// Get the newly created review to return as HTML
	reviews, err := h.getReviewsByMerchantID(merchantID)
	if err != nil || len(reviews) == 0 {
		log.Printf("AddReview error: Failed to retrieve created review - %v", err)
		c.Header("Content-Type", "text/html")
		c.String(http.StatusInternalServerError, `<script>
			iziToast.error({
				title: 'Error',
				message: 'Failed to retrieve the created template',
				icon: 'fas fa-exclamation-circle',
			});
		</script>`)
		return
	}

	// Get the last review (the one we just created)
	newReview := reviews[len(reviews)-1]

	// Return HTML for the new review item with success toast
	html := fmt.Sprintf(`
		<div class="review-item border border-gray-200 rounded-lg p-4 mb-4" data-review-id="%d">
			<div class="flex justify-between items-start mb-3">
				<div class="flex items-center space-x-3">
					<select name="platform_%d" class="review-platform border-gray-300 rounded-md text-sm">
						<option value="google" %s>Google</option>
						<option value="facebook" %s>Facebook</option>
					</select>
					<span class="text-sm text-gray-600">Template</span>
				</div>
				<div class="flex items-center space-x-2">
					<label class="flex items-center">
						<input type="checkbox" name="is_active_%d" %s class="review-active">
						<span class="ml-2 text-sm text-gray-600">Active</span>
					</label>
					<button type="button" class="text-red-600 hover:text-red-800 text-sm"
							hx-delete="/api/reviews/%d"
							hx-target="closest .review-item"
							hx-swap="outerHTML"
							hx-confirm="Are you sure you want to delete this review template?">Delete</button>
				</div>
			</div>
			<div class="space-y-3">
				<textarea name="text_%d" rows="3" placeholder="Review template text that customers can copy..." class="block w-full border-gray-300 rounded-md shadow-sm text-sm">%s</textarea>
			</div>
		</div>
		<script>
			iziToast.success({
				title: 'Template Added!',
				message: 'Review template has been created successfully.',
				icon: 'fas fa-plus-circle',
			});
		</script>`,
		newReview.ID,
		newReview.ID,
		func() string { if newReview.Platform == "google" { return "selected" } else { return "" } }(),
		func() string { if newReview.Platform == "facebook" { return "selected" } else { return "" } }(),
		newReview.ID,
		func() string { if newReview.IsActive { return "checked" } else { return "" } }(),
		newReview.ID,
		newReview.ID,
		template.JSEscapeString(newReview.ReviewText),
	)

	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, html)
}

func (h *Handlers) DeleteReview(c *gin.Context) {
	reviewIDStr := c.Param("id")
	reviewID, err := strconv.Atoi(reviewIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid review ID"})
		return
	}

	err = h.deleteReview(reviewID)
	if err != nil {
		c.Header("Content-Type", "text/html")
		c.String(http.StatusInternalServerError, `<script>
			iziToast.error({
				title: 'Error',
				message: 'Failed to delete review template',
				icon: 'fas fa-exclamation-circle',
			});
		</script>`)
		return
	}

	// Return empty response with success toast (HTMX will remove the element)
	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, `<script>
		iziToast.success({
			title: 'Template Deleted!',
			message: 'Review template has been deleted successfully.',
			icon: 'fas fa-trash-alt',
		});
	</script>`)
}

// GetReviewsData returns reviews data as JSON for a specific merchant
func (h *Handlers) GetReviewsData(c *gin.Context) {
	merchantIDStr := c.Param("merchantId")
	merchantID, err := strconv.Atoi(merchantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid merchant ID"})
		return
	}

	// Get active reviews for this merchant
	reviews, err := h.getActiveReviewsByMerchantID(merchantID)
	if err != nil {
		log.Printf("Failed to fetch reviews for merchant %d: %v", merchantID, err)
		reviews = []Review{} // Empty slice if error
	}

	// Group reviews by platform for the frontend
	reviewsData := map[string][]map[string]interface{}{
		"google":   make([]map[string]interface{}, 0),
		"facebook": make([]map[string]interface{}, 0),
	}

	for _, review := range reviews {
		reviewItem := map[string]interface{}{
			"id":     review.ID,
			"text":   review.ReviewText,
		}

		if review.Platform == "google" {
			reviewsData["google"] = append(reviewsData["google"], reviewItem)
		} else if review.Platform == "facebook" {
			reviewsData["facebook"] = append(reviewsData["facebook"], reviewItem)
		}
	}

	c.JSON(http.StatusOK, reviewsData)
}

// GetReviewModal returns HTML content for the review modal
func (h *Handlers) GetReviewModal(c *gin.Context) {
	merchantIDStr := c.Param("merchantId")
	platform := c.Param("platform")

	merchantID, err := strconv.Atoi(merchantIDStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid merchant ID")
		return
	}

	// Get active reviews for this merchant and platform
	reviews, err := h.getActiveReviewsByMerchantID(merchantID)
	if err != nil {
		log.Printf("Failed to fetch reviews for merchant %d: %v", merchantID, err)
		reviews = []Review{}
	}

	// Filter by platform
	var platformReviews []Review
	for _, review := range reviews {
		if review.Platform == platform {
			platformReviews = append(platformReviews, review)
		}
	}

	// Get merchant and business details for URLs
	merchant, _ := h.getMerchantByID(merchantID)
	details, _ := h.getMerchantDetails(merchantID)

	// Generate HTML content
	html := fmt.Sprintf(`
		<div class="modal-header">
			<h5 class="modal-title">%s Reviews</h5>
			<button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
		</div>
		<div class="modal-body">
			<div class="mb-4">
	`, strings.Title(platform))

	if len(platformReviews) == 0 {
		html += `<div class="text-center py-4"><p class="text-muted">No review templates available.</p></div>`
	} else {
		for _, review := range platformReviews {
			html += fmt.Sprintf(`
				<div class="card mb-3">
					<div class="input-group">
						<input type="text" class="form-control" value="%s" readonly onclick="copyAndRedirect('%s', '%s')">
						<button class="btn btn-outline-secondary" type="button" onclick="copyAndRedirect('%s', '%s')">
							<i class="fas fa-copy"></i>
						</button>
					</div>
				</div>
			`, review.ReviewText, review.ReviewText, platform, review.ReviewText, platform)
		}
	}

	// Add write review button
	writeURL := ""
	if platform == "google" {
		if details.Address != "" {
			writeURL = fmt.Sprintf("https://www.google.com/maps/search/%s", url.QueryEscape(details.Address))
		} else if merchant != nil {
			writeURL = fmt.Sprintf("https://www.google.com/maps/search/%s", url.QueryEscape(merchant.BusinessName))
		}
	} else if platform == "facebook" {
		if details.FacebookURL != "" {
			writeURL = details.FacebookURL
		} else if merchant != nil {
			writeURL = fmt.Sprintf("https://www.facebook.com/search/top?q=%s", url.QueryEscape(merchant.BusinessName))
		}
	}

	html += fmt.Sprintf(`
			</div>
			<div class="d-grid">
				<button class="btn btn-primary" onclick="window.open('%s', '_blank')">
					<i class="fas fa-edit me-2"></i>Write a Review
				</button>
			</div>
		</div>
	`, writeURL)

	c.Header("Content-Type", "text/html")
	c.String(http.StatusOK, html)
}

// logAuditEvent logs an admin action to the audit_logs table
func (h *Handlers) logAuditEvent(c *gin.Context, action, targetType, targetID string, details map[string]interface{}) {
	// Get admin user info from context (set by middleware)
	userID, _ := c.Get("user_id")
	userEmail, _ := c.Get("user_email")

	// Get IP address
	ipAddress := c.ClientIP()

	// Get user agent
	userAgent := c.GetHeader("User-Agent")

	// Convert details to JSONB
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		log.Printf("Failed to marshal audit details: %v", err)
		detailsJSON = []byte("{}")
	}

	// Insert audit log
	_, err = h.db.Exec(`
		INSERT INTO audit_logs (user_id, user_email, action, target_type, target_id, details, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, userID, userEmail, action, targetType, targetID, detailsJSON, ipAddress, userAgent)

	if err != nil {
		log.Printf("Failed to create audit log: %v", err)
		// Don't fail the request if audit logging fails
	} else {
		log.Printf("Audit log created: %s by %s on %s:%s", action, userEmail, targetType, targetID)
	}
}

// Analytics tracking endpoints

// TrackPageView logs a page view for analytics
func (h *Handlers) TrackPageView(c *gin.Context) {
	merchantIDStr := c.Query("merchant_id")
	if merchantIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_id required"})
		return
	}

	merchantID, err := strconv.Atoi(merchantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant_id"})
		return
	}

	// Get tracking data
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	referrer := c.GetHeader("Referer")

	// Insert page view
	_, err = h.db.Exec(`
		INSERT INTO page_views (merchant_id, ip_address, user_agent, referrer)
		VALUES ($1, $2, $3, $4)
	`, merchantID, ipAddress, userAgent, referrer)

	if err != nil {
		log.Printf("Failed to log page view: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to track view"})
		return
	}

	log.Printf("Page view tracked: merchant_id=%d, ip=%s", merchantID, ipAddress)
	c.JSON(http.StatusOK, gin.H{"status": "tracked"})
}

// TrackLinkClick logs a link click for analytics
func (h *Handlers) TrackLinkClick(c *gin.Context) {
	merchantIDStr := c.Query("merchant_id")
	platform := c.Query("platform")
	linkType := c.Query("type")

	if merchantIDStr == "" || platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_id and platform required"})
		return
	}

	merchantID, err := strconv.Atoi(merchantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merchant_id"})
		return
	}

	// Default link type to 'social' if not specified
	if linkType == "" {
		linkType = "social"
	}

	// Get tracking data
	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	// Insert link click
	_, err = h.db.Exec(`
		INSERT INTO link_clicks (merchant_id, platform, link_type, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5)
	`, merchantID, platform, linkType, ipAddress, userAgent)

	if err != nil {
		log.Printf("Failed to log link click: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to track click"})
		return
	}

	log.Printf("Link click tracked: merchant_id=%d, platform=%s, type=%s", merchantID, platform, linkType)
	c.JSON(http.StatusOK, gin.H{"status": "tracked"})
}
