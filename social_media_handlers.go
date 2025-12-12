package main

import (
	"auto-gbp-review/social_media"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

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

	// Get merchant ID from authenticated user
	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Merchant not found"})
		return
	}

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
	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.String(http.StatusUnauthorized, "Merchant not found")
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
		c.String(http.StatusInternalServerError, "Failed to get account information")
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

	// Clear cookies
	c.SetCookie("oauth_state", "", -1, "/", "", false, true)
	c.SetCookie("oauth_platform", "", -1, "/", "", false, true)

	// Trigger initial sync
	go func() {
		h.syncService.SyncConnection(connection.ID, socialmedia.SyncTypeManual)
	}()

	// Redirect to dashboard
	c.Redirect(http.StatusTemporaryRedirect, "/dashboard/integrations")
}

// GetConnections returns all API connections for the merchant
func (h *SocialMediaHandlers) GetConnections(c *gin.Context) {
	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Merchant not found"})
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

	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Merchant not found"})
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

	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Merchant not found"})
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
	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Merchant not found"})
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
	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.Redirect(http.StatusTemporaryRedirect, "/login")
		return
	}

	smDB := socialmedia.NewDB(h.db.DB)
	connections, _ := smDB.GetAPIConnectionsByMerchant(merchantID)

	renderPage(c, "templates/layouts/base.html", "templates/merchant/integrations.html", gin.H{
		"title":       "Social Media Integrations",
		"connections": connections,
		"platforms": map[string]bool{
			"google_business": os.Getenv("GOOGLE_CLIENT_ID") != "",
			"facebook":        os.Getenv("FACEBOOK_APP_ID") != "",
			"instagram":       os.Getenv("FACEBOOK_APP_ID") != "",
		},
	})
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

	merchantID := c.GetInt("merchant_id")
	if merchantID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Merchant not found"})
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
