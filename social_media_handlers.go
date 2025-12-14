package main

import (
	"auto-gbp-review/social_media"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// setFlashCookies sets flash message cookies with consistent parameters
func setFlashCookies(c *gin.Context, flashType, platform, message string) {
	c.SetCookie("oauth_error_type", flashType, 300, "/", "", false, true)
	c.SetCookie("oauth_platform", platform, 300, "/", "", false, true)
	c.SetCookie("oauth_error_msg", message, 300, "/", "", false, true)
}

// setSuccessCookie sets success flash cookie
func setSuccessCookie(c *gin.Context, platform, message string) {
	c.SetCookie("oauth_success", message, 300, "/", "", false, true)
	c.SetCookie("oauth_platform", platform, 300, "/", "", false, true)
}

// clearFlashCookies clears all flash message cookies
func clearFlashCookies(c *gin.Context) {
	cookies := []string{"oauth_error_type", "oauth_platform", "oauth_error_msg", "oauth_success"}
	for _, cookie := range cookies {
		c.SetCookie(cookie, "", -1, "/", "", false, true)
	}
}

// SocialMediaHandlers handles OAuth and sync operations for social media integrations
type SocialMediaHandlers struct {
	db          *Database
	syncService *socialmedia.SyncService
	scheduler   *socialmedia.Scheduler
	providers   map[string]socialmedia.SocialMediaProvider
}

// NewSocialMediaHandlers creates a new social media handlers instance
func NewSocialMediaHandlers(db *Database) *SocialMediaHandlers {
	// Initialize encryption
	encryptionKey := socialmedia.EncryptionKeyFromString(os.Getenv("ENCRYPTION_KEY"))
	encryptor, err := socialmedia.NewAESEncryptor(encryptionKey)
	if err != nil {
		log.Fatal("Failed to initialize encryptor:", err)
	}

	// Initialize social media database
	smDB := socialmedia.NewDB(db.DB)

	// Create sync service
	syncService := socialmedia.NewSyncService(smDB, encryptor)

	// Initialize providers
	providers := make(map[string]socialmedia.SocialMediaProvider)

	// Google Business Profile
	if os.Getenv("GOOGLE_CLIENT_ID") != "" {
		gbProvider := socialmedia.NewGoogleBusinessProvider(
			os.Getenv("GOOGLE_CLIENT_ID"),
			os.Getenv("GOOGLE_CLIENT_SECRET"),
			os.Getenv("GOOGLE_REDIRECT_URI"),
		)
		providers[socialmedia.PlatformGoogleBusiness] = gbProvider
		syncService.RegisterProvider(gbProvider)
	}

	// Facebook
	if os.Getenv("FACEBOOK_APP_ID") != "" {
		fbProvider := socialmedia.NewFacebookProvider(
			os.Getenv("FACEBOOK_APP_ID"),
			os.Getenv("FACEBOOK_APP_SECRET"),
			os.Getenv("FACEBOOK_REDIRECT_URI"),
		)
		providers[socialmedia.PlatformFacebook] = fbProvider
		syncService.RegisterProvider(fbProvider)
	}

	// Instagram (uses same credentials as Facebook)
	if os.Getenv("FACEBOOK_APP_ID") != "" {
		igProvider := socialmedia.NewInstagramProvider(
			os.Getenv("FACEBOOK_APP_ID"),
			os.Getenv("FACEBOOK_APP_SECRET"),
			os.Getenv("FACEBOOK_REDIRECT_URI"),
		)
		providers[socialmedia.PlatformInstagram] = igProvider
		syncService.RegisterProvider(igProvider)
	}

	// Xiaohongshu (XHS)
	if os.Getenv("XHS_APP_KEY") != "" {
		xhsProvider := socialmedia.NewXHSProvider(
			os.Getenv("XHS_APP_KEY"),
			os.Getenv("XHS_APP_SECRET"),
			os.Getenv("XHS_REDIRECT_URI"),
		)
		providers[socialmedia.PlatformXiaohongshu] = xhsProvider
		syncService.RegisterProvider(xhsProvider)
	}

	// Create scheduler
	scheduler := socialmedia.NewScheduler(syncService)
	scheduler.Start()

	return &SocialMediaHandlers{
		db:          db,
		syncService: syncService,
		scheduler:   scheduler,
		providers:   providers,
	}
}

// generateState generates a random state string for OAuth
func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// ConnectPlatform initiates OAuth flow for a platform
func (h *SocialMediaHandlers) ConnectPlatform(c *gin.Context) {
	platform := c.Param("platform")

	// Check if provider exists
	provider, ok := h.providers[platform]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported platform"})
		return
	}

	// Generate state for CSRF protection
	state := generateState()

	// Store state in session (you should use a proper session store)
	c.SetCookie("oauth_state", state, 3600, "/", "", false, true)
	c.SetCookie("oauth_platform", platform, 3600, "/", "", false, true)

	// Redirect to OAuth authorization URL
	authURL := provider.GetAuthorizationURL(state)
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// OAuthCallback handles OAuth callback from providers
func (h *SocialMediaHandlers) OAuthCallback(c *gin.Context) {
	platform := c.Param("platform")

	// Debug: Log incoming request
	log.Printf("OAuth callback for platform: %s", platform)
	log.Printf("Full URL: %s", c.Request.URL.String())

	// Get merchant ID from authenticated user (already validated above)

	// Verify state
	stateCookie, _ := c.Cookie("oauth_state")
	stateParam := c.Query("state")

	if stateCookie == "" || stateCookie != stateParam {
		c.String(http.StatusBadRequest, "Invalid state parameter")
		return
	}

	// Get authorization code
	code := c.Query("code")
	if code == "" {
		errorDesc := c.Query("error_description")
		c.String(http.StatusBadRequest, "Authorization failed: %s", errorDesc)
		return
	}

	// Get merchant ID from authenticated user
	merchantID, err := h.getMerchantIDFromContext(c)
	if err != nil {
		c.String(http.StatusUnauthorized, err.Error())
		return
	}

	// Get provider
	provider, ok := h.providers[platform]
	if !ok {
		c.String(http.StatusBadRequest, "Unsupported platform")
		return
	}

	// Exchange code for tokens
	tokenResp, err := provider.ExchangeCodeForToken(code)
	if err != nil {
		log.Printf("Error exchanging code for token: %v", err)
		c.String(http.StatusInternalServerError, "Failed to exchange authorization code")
		return
	}

	// Get account info
	accountInfo, err := provider.GetAccountInfo(tokenResp.AccessToken)
	if err != nil {
		log.Printf("Error getting account info: %v", err)

		// Categorize the error
		errorType := categorizeError(err, platform)
		errorMsg := getErrorMessage(errorType, err)

		// Store error in cookies using helper function
		setFlashCookies(c, errorType, platform, errorMsg)

		// Redirect to clean URL without query parameters
		c.Redirect(http.StatusTemporaryRedirect, "/dashboard/integrations")
		return
	}

	// Encrypt tokens
	encryptionKey := socialmedia.EncryptionKeyFromString(os.Getenv("ENCRYPTION_KEY"))
	encryptor, _ := socialmedia.NewAESEncryptor(encryptionKey)

	encryptedAccess, err := encryptor.Encrypt(tokenResp.AccessToken)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to encrypt tokens")
		return
	}

	encryptedRefresh := ""
	if tokenResp.RefreshToken != "" {
		encryptedRefresh, _ = encryptor.Encrypt(tokenResp.RefreshToken)
	}

	// Save API connection
	smDB := socialmedia.NewDB(h.db.DB)
	connection := &socialmedia.APIConnection{
		MerchantID:          merchantID,
		Platform:            platform,
		PlatformAccountID:   accountInfo.AccountID,
		PlatformAccountName: accountInfo.AccountName,
		AccessToken:         encryptedAccess,
		RefreshToken:        encryptedRefresh,
		TokenExpiresAt:      tokenResp.ExpiresAt,
		IsActive:            true,
		SyncStatus:          socialmedia.SyncStatusPending,
	}

	err = smDB.CreateAPIConnection(connection)
	if err != nil {
		log.Printf("Error saving API connection: %v", err)
		c.String(http.StatusInternalServerError, "Failed to save connection")
		return
	}

	// Clear OAuth state cookies (keep flash cookies for display)
	c.SetCookie("oauth_state", "", -1, "/", "", false, true)

	// Trigger initial sync
	go func() {
		h.syncService.SyncConnection(connection.ID, socialmedia.SyncTypeManual)
	}()

	// Store success in cookies using helper function
	setSuccessCookie(c, platform, "connected")

	// Redirect to clean URL without query parameters
	c.Redirect(http.StatusTemporaryRedirect, "/dashboard/integrations")
}

// GetConnections returns all API connections for the merchant
func (h *SocialMediaHandlers) GetConnections(c *gin.Context) {
	merchantID, err := h.getMerchantIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	smDB := socialmedia.NewDB(h.db.DB)
	connections, err := smDB.GetAPIConnectionsByMerchant(merchantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get connections"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"connections": connections})
}

// DisconnectPlatform removes an API connection
func (h *SocialMediaHandlers) DisconnectPlatform(c *gin.Context) {
	connectionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid connection ID"})
		return
	}

	merchantID, err := h.getMerchantIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	smDB := socialmedia.NewDB(h.db.DB)

	// Verify connection belongs to merchant
	connection, err := smDB.GetAPIConnection(connectionID)
	if err != nil || connection.MerchantID != merchantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Connection not found"})
		return
	}

	err = smDB.DeleteAPIConnection(connectionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete connection"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Connection removed successfully"})
}

// TriggerSync manually triggers a sync for a connection
func (h *SocialMediaHandlers) TriggerSync(c *gin.Context) {
	connectionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid connection ID"})
		return
	}

	merchantID, err := h.getMerchantIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	smDB := socialmedia.NewDB(h.db.DB)

	// Verify connection belongs to merchant
	connection, err := smDB.GetAPIConnection(connectionID)
	if err != nil || connection.MerchantID != merchantID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Connection not found"})
		return
	}

	// Trigger sync
	stats, err := h.syncService.SyncConnection(connectionID, socialmedia.SyncTypeManual)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Sync failed",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Sync completed",
		"stats": gin.H{
			"fetched": stats.TotalFetched,
			"added":   stats.TotalAdded,
			"updated": stats.TotalUpdated,
		},
	})
}

// GetSyncedReviews returns synced reviews for the merchant
func (h *SocialMediaHandlers) GetSyncedReviews(c *gin.Context) {
	// Get merchant ID from authenticated user
	merchantID, err := h.getMerchantIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Get pagination params
	limit := 50
	offset := 0

	if limitParam := c.Query("limit"); limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil {
			limit = l
		}
	}

	if offsetParam := c.Query("offset"); offsetParam != "" {
		if o, err := strconv.Atoi(offsetParam); err == nil {
			offset = o
		}
	}

	smDB := socialmedia.NewDB(h.db.DB)
	reviews, err := smDB.GetSyncedReviewsByMerchant(merchantID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get reviews"})
		return
	}

	// Get stats
	stats, _ := smDB.GetMerchantReviewStats(merchantID)

	c.JSON(http.StatusOK, gin.H{
		"reviews": reviews,
		"stats":   stats,
	})
}

// IntegrationsPage renders the integrations management page
func (h *SocialMediaHandlers) IntegrationsPage(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.Redirect(http.StatusTemporaryRedirect, "/login")
		return
	}

	// Get merchants for this user
	merchants, err := h.getMerchantsByAuthUserID(userID)
	if err != nil || len(merchants) == 0 {
		renderPage(c, "templates/layouts/base.html", "templates/error.html", gin.H{
			"error": "No business account found. Please create a business profile first.",
		})
		return
	}

	// Use first merchant (most users have one business)
	merchantID := merchants[0].ID

	smDB := socialmedia.NewDB(h.db.DB)
	connections, _ := smDB.GetAPIConnectionsByMerchant(merchantID)

	// Check for error or success in cookies first, then query parameters (fallback)
	var errorType, platform, errorMsg, success string

	// Try cookies first (new approach)
	errorType, _ = c.Cookie("oauth_error_type")
	errorMsg, _ = c.Cookie("oauth_error_msg")
	platform, _ = c.Cookie("oauth_platform")
	success, _ = c.Cookie("oauth_success")

	// Fallback to query parameters (for direct access or old links)
	if errorType == "" && c.Query("error") != "" {
		errorType = c.Query("error")
		errorMsg = c.Query("msg")
		platform = c.Query("platform")
	}
	if success == "" && c.Query("success") != "" {
		success = c.Query("success")
		if platform == "" {
			platform = c.Query("platform")
		}
	}

	// Prepare data for template
	data := gin.H{
		"title":       "Social Media Integrations",
		"connections": connections,
		"platforms": map[string]bool{
			"google_business": os.Getenv("GOOGLE_CLIENT_ID") != "",
			"facebook":        os.Getenv("FACEBOOK_APP_ID") != "",
			"instagram":       os.Getenv("FACEBOOK_APP_ID") != "",
			"xiaohongshu":     os.Getenv("XHS_APP_KEY") != "",
		},
	}

	// Add error data if present
	if errorType != "" {
		data["errorType"] = errorType
		data["errorPlatform"] = platform
		data["errorMessage"] = errorMsg
	}

	// Add success data if present
	if success != "" {
		data["success"] = success
		data["platform"] = platform
	}

	// Clear flash cookies after reading them (show-once behavior)
	clearFlashCookies(c)

	renderPage(c, "templates/layouts/base.html", "templates/merchant/integrations.html", data)
}

// getMerchantIDFromContext gets the merchant ID from the authenticated user context
func (h *SocialMediaHandlers) getMerchantIDFromContext(c *gin.Context) (int, error) {
	userID := c.GetString("user_id")
	if userID == "" {
		return 0, fmt.Errorf("user not authenticated")
	}

	merchants, err := h.getMerchantsByAuthUserID(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get merchants: %w", err)
	}

	if len(merchants) == 0 {
		return 0, fmt.Errorf("no business account found")
	}

	// Use first merchant (most users have one business)
	return merchants[0].ID, nil
}

// getMerchantsByAuthUserID retrieves merchants for a given auth user ID
func (h *SocialMediaHandlers) getMerchantsByAuthUserID(authUserID string) ([]Merchant, error) {
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

// AdminConnectionsPage shows all connections for admin
func (h *SocialMediaHandlers) AdminConnectionsPage(c *gin.Context) {
	// This would show all connections across all merchants for admin monitoring
	c.String(http.StatusOK, "Admin connections page - TODO")
}

// GetSyncLogs returns sync logs for a connection
func (h *SocialMediaHandlers) GetSyncLogs(c *gin.Context) {
	connectionID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid connection ID"})
		return
	}

	merchantID, err := h.getMerchantIDFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	smDB := socialmedia.NewDB(h.db.DB)

	// Verify connection belongs to merchant (unless admin)
	role := c.GetString("role")
	if role != "admin" {
		connection, err := smDB.GetAPIConnection(connectionID)
		if err != nil || connection.MerchantID != merchantID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Connection not found"})
			return
		}
	}

	logs, err := smDB.GetSyncLogsByConnection(connectionID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// categorizeError categorizes OAuth errors for better user messages
func categorizeError(err error, platform string) string {
	errStr := err.Error()

	if strings.Contains(errStr, "429") || strings.Contains(errStr, "Too Many Requests") {
		return "rate_limit"
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
		return "auth"
	}
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "Forbidden") {
		return "permission"
	}
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		if platform == "google_business" {
			return "no_business"
		}
		return "not_found"
	}
	if strings.Contains(errStr, "network") || strings.Contains(errStr, "connection") {
		return "network"
	}

	return "unknown"
}

// getErrorMessage returns user-friendly error messages
func getErrorMessage(errorType string, err error) string {
	switch errorType {
	case "rate_limit":
		return "API quota exceeded. Please wait a few minutes before trying again."
	case "auth":
		return "Authentication failed. Please try connecting again."
	case "permission":
		return "Permission denied. Make sure you have admin access to the business account."
	case "no_business":
		return "No Google Business Profile found. Please create or claim a business profile first."
	case "not_found":
		return "The requested resource was not found. Please contact support if this persists."
	case "network":
		return "Network error. Please check your internet connection and try again."
	default:
		return fmt.Sprintf("Connection failed: %v", err)
	}
}
