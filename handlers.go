package main

import (
	"auto-gbp-review/utils"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
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
		"title": "Auto GBP Review System",
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

	// Hardcoded reviews for demonstration
	reviews := []Review{
		{ID: 1, Platform: "google", Author: "John Doe", Text: "Excellent service! Highly recommended.", Rating: 5},
		{ID: 2, Platform: "google", Author: "Jane Smith", Text: "Great experience, will come back again.", Rating: 5},
		{ID: 3, Platform: "facebook", Author: "Bob Wilson", Text: "Outstanding quality and friendly staff.", Rating: 4},
		{ID: 4, Platform: "facebook", Author: "Alice Brown", Text: "Very satisfied with the service provided.", Rating: 5},
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

func (h *Handlers) Login(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")

	user, err := h.getUserByEmail(email)
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/login.html", gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/login.html", gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	// Generate JWT token
	token, err := GenerateJWT(user.ID, user.Email, user.Role)
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/login.html", gin.H{
			"error": "Login failed",
		})
		return
	}

	// Set cookie
	c.SetCookie("auth_token", token, 86400*7, "/", "", false, true) // 7 days

	// Redirect based on role
	if user.Role == "admin" {
		c.Redirect(http.StatusFound, "/admin")
	} else {
		c.Redirect(http.StatusFound, "/dashboard")
	}
}

func (h *Handlers) RegisterPage(c *gin.Context) {
	renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
		"title": "Register",
	})
}

func (h *Handlers) Register(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")
	confirmPassword := c.PostForm("confirm_password")

	if password != confirmPassword {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": "Passwords do not match",
		})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": "Registration failed",
		})
		return
	}

	// Create user
	userID, err := h.createUser(email, string(hashedPassword), "merchant")
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": "Email already exists",
		})
		return
	}

	// Generate JWT token
	token, err := GenerateJWT(userID, email, "merchant")
	if err != nil {
		renderPage(c, "templates/layouts/auth.html", "templates/auth/register.html", gin.H{
			"error": "Registration failed",
		})
		return
	}

	// Set cookie
	c.SetCookie("auth_token", token, 86400*7, "/", "", false, true)

	c.Redirect(http.StatusFound, "/dashboard")
}

func (h *Handlers) Logout(c *gin.Context) {
	c.SetCookie("auth_token", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/")
}

// Admin handlers
func (h *Handlers) AdminDashboard(c *gin.Context) {
	renderPage(c, "templates/layouts/base.html", "templates/admin/dashboard.html", gin.H{
		"title": "Admin Dashboard",
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

func (h *Handlers) AdminMerchantForm(c *gin.Context) {
	renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_form.html", gin.H{
		"title": "Add New Merchant",
	})
}

func (h *Handlers) AdminCreateMerchant(c *gin.Context) {
	businessName := c.PostForm("business_name")
	slug := c.PostForm("slug")
	userEmail := c.PostForm("user_email")

	// Get user ID by email
	user, err := h.getUserByEmail(userEmail)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/admin/merchant_form.html", gin.H{
			"title": "Add New Merchant",
			"error": "User not found",
		})
		return
	}

	// Create merchant
	merchantID, err := h.createMerchant(user.ID, businessName, slug)
	if err != nil {
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
	userID := c.GetInt("user_id")
	merchants, err := h.getMerchantsByUserID(userID)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load your businesses",
		})
		return
	}

	renderPage(c, "templates/layouts/base.html", "templates/merchant_dashboard.html", gin.H{
		"title":     "Dashboard",
		"merchants": merchants,
	})
}

func (h *Handlers) MerchantProfile(c *gin.Context) {
	userID := c.GetInt("user_id")
	merchants, err := h.getMerchantsByUserID(userID)
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

	renderPage(c, "templates/layouts/base.html", "templates/merchant_profile.html", gin.H{
		"title":    "Profile",
		"merchant": merchant,
		"details":  details,
	})
}

// Replace your existing UpdateMerchantProfile function in handlers.go with this:
func (h *Handlers) UpdateMerchantProfile(c *gin.Context) {
	userID := c.GetInt("user_id")

	// Get or create merchant (your existing logic)
	merchants, err := h.getMerchantsByUserID(userID)
	if err != nil {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "Failed to load your business",
		})
		return
	}

	var merchantID int
	var currentDetails *MerchantDetails

	if len(merchants) == 0 {
		// Create new merchant
		businessName := c.PostForm("business_name")
		slug := c.PostForm("slug")
		merchantID, err = h.createMerchant(userID, businessName, slug)
		if err != nil {
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
		businessName := c.PostForm("business_name")
		slug := c.PostForm("slug")
		err = h.updateMerchant(merchantID, businessName, slug, true)
		if err != nil {
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
			// Get existing data for redisplay
			merchants, _ := h.getMerchantsByUserID(userID)
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
			merchants, _ := h.getMerchantsByUserID(userID)
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

	c.Redirect(http.StatusFound, "/dashboard/profile?success=1")
}

func (h *Handlers) ToggleMerchantStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid merchant ID"})
		return
	}

	err = h.toggleMerchantStatus(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle status"})
		return
	}

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
	UserID       int       `json:"user_id"`
	BusinessName string    `json:"business_name"`
	Slug         string    `json:"slug"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UserEmail    string    `json:"user_email,omitempty"` // For admin views
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
	ID       int    `json:"id"`
	Platform string `json:"platform"`
	Author   string `json:"author"`
	Text     string `json:"text"`
	Rating   int    `json:"rating"`
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
	err := h.db.QueryRow("SELECT id, user_id, business_name, slug, is_active, created_at FROM merchants WHERE id = $1", id).
		Scan(&merchant.ID, &merchant.UserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt)
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

func (h *Handlers) createUser(email, passwordHash, role string) (int, error) {
	var userID int
	err := h.db.QueryRow("INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3) RETURNING id",
		email, passwordHash, role).Scan(&userID)
	return userID, err
}

func (h *Handlers) getMerchantBySlug(slug string) (*Merchant, error) {
	merchant := &Merchant{}
	err := h.db.QueryRow("SELECT id, user_id, business_name, slug, is_active, created_at FROM merchants WHERE slug = $1 AND is_active = true", slug).
		Scan(&merchant.ID, &merchant.UserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt)
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
	rows, err := h.db.Query("SELECT id, user_id, business_name, slug, is_active, created_at FROM merchants ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merchants []Merchant
	for rows.Next() {
		var merchant Merchant
		if err := rows.Scan(&merchant.ID, &merchant.UserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt); err != nil {
			return nil, err
		}
		merchants = append(merchants, merchant)
	}
	return merchants, nil
}

func (h *Handlers) getAllMerchantsWithDetails() ([]Merchant, error) {
	rows, err := h.db.Query(`SELECT m.id, m.user_id, m.business_name, m.slug, m.is_active, m.created_at, u.email 
		FROM merchants m JOIN users u ON m.user_id = u.id ORDER BY m.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merchants []Merchant
	for rows.Next() {
		var merchant Merchant
		if err := rows.Scan(&merchant.ID, &merchant.UserID, &merchant.BusinessName, &merchant.Slug,
			&merchant.IsActive, &merchant.CreatedAt, &merchant.UserEmail); err != nil {
			return nil, err
		}
		merchants = append(merchants, merchant)
	}
	return merchants, nil
}

func (h *Handlers) getMerchantsByUserID(userID int) ([]Merchant, error) {
	rows, err := h.db.Query("SELECT id, user_id, business_name, slug, is_active, created_at FROM merchants WHERE user_id = $1 ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merchants []Merchant
	for rows.Next() {
		var merchant Merchant
		if err := rows.Scan(&merchant.ID, &merchant.UserID, &merchant.BusinessName, &merchant.Slug, &merchant.IsActive, &merchant.CreatedAt); err != nil {
			return nil, err
		}
		merchants = append(merchants, merchant)
	}
	return merchants, nil
}
