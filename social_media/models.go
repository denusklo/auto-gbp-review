package socialmedia

import (
	"database/sql"
	"time"
)

// APIConnection represents a connection to a social media platform
type APIConnection struct {
	ID                  int       `json:"id"`
	MerchantID          int       `json:"merchant_id"`
	Platform            string    `json:"platform"` // 'google_business', 'facebook', 'instagram'
	PlatformAccountID   string    `json:"platform_account_id"`
	PlatformAccountName string    `json:"platform_account_name"`
	AccessToken         string    `json:"-"` // Don't serialize to JSON
	RefreshToken        string    `json:"-"` // Don't serialize to JSON
	TokenExpiresAt      time.Time `json:"token_expires_at"`
	IsActive            bool      `json:"is_active"`
	LastSyncAt          *time.Time `json:"last_sync_at"`
	SyncStatus          string    `json:"sync_status"` // 'pending', 'syncing', 'completed', 'failed'
	ErrorMessage        string    `json:"error_message,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// SyncedReview represents a review synced from a social media platform
type SyncedReview struct {
	ID               int            `json:"id"`
	MerchantID       int            `json:"merchant_id"`
	APIConnectionID  *int           `json:"api_connection_id"`
	Platform         string         `json:"platform"`
	PlatformReviewID string         `json:"platform_review_id"`
	AuthorName       string         `json:"author_name"`
	AuthorPhotoURL   string         `json:"author_photo_url,omitempty"`
	Rating           *float64       `json:"rating"`
	ReviewText       string         `json:"review_text"`
	ReviewReply      string         `json:"review_reply,omitempty"`
	ReviewedAt       time.Time      `json:"reviewed_at"`
	SyncedAt         time.Time      `json:"synced_at"`
	IsVisible        bool           `json:"is_visible"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// SyncLog represents a log entry for a sync operation
type SyncLog struct {
	ID              int       `json:"id"`
	APIConnectionID int       `json:"api_connection_id"`
	SyncType        string    `json:"sync_type"` // 'manual', 'scheduled', 'webhook'
	Status          string    `json:"status"`    // 'started', 'completed', 'failed'
	ReviewsFetched  int       `json:"reviews_fetched"`
	ReviewsAdded    int       `json:"reviews_added"`
	ReviewsUpdated  int       `json:"reviews_updated"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

// TokenResponse represents an OAuth token response
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int       `json:"expires_in"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Review represents a review from any platform (normalized)
type Review struct {
	PlatformReviewID string                 `json:"platform_review_id"`
	AuthorName       string                 `json:"author_name"`
	AuthorPhotoURL   string                 `json:"author_photo_url,omitempty"`
	Rating           *float64               `json:"rating"`
	ReviewText       string                 `json:"review_text"`
	ReviewReply      string                 `json:"review_reply,omitempty"`
	ReviewedAt       time.Time              `json:"reviewed_at"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// AccountInfo represents account information from a platform
type AccountInfo struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// SyncStats represents statistics from a sync operation
type SyncStats struct {
	TotalFetched int
	TotalAdded   int
	TotalUpdated int
	Errors       []error
}

// Platform constants
const (
	PlatformGoogleBusiness = "google_business"
	PlatformFacebook       = "facebook"
	PlatformInstagram      = "instagram"
	PlatformXiaohongshu    = "xiaohongshu"
)

// Sync status constants
const (
	SyncStatusPending   = "pending"
	SyncStatusSyncing   = "syncing"
	SyncStatusCompleted = "completed"
	SyncStatusFailed    = "failed"
)

// Sync type constants
const (
	SyncTypeManual    = "manual"
	SyncTypeScheduled = "scheduled"
	SyncTypeWebhook   = "webhook"
)

// Database interface for social media operations
type SocialMediaDB interface {
	// API Connections
	CreateAPIConnection(conn *APIConnection) error
	GetAPIConnection(id int) (*APIConnection, error)
	GetAPIConnectionsByMerchant(merchantID int) ([]*APIConnection, error)
	GetAPIConnectionByPlatform(merchantID int, platform string) (*APIConnection, error)
	UpdateAPIConnection(conn *APIConnection) error
	DeleteAPIConnection(id int) error
	GetActiveConnections() ([]*APIConnection, error)

	// Synced Reviews
	CreateSyncedReview(review *SyncedReview) error
	GetSyncedReview(id int) (*SyncedReview, error)
	GetSyncedReviewByPlatformID(platform, platformReviewID string) (*SyncedReview, error)
	GetSyncedReviewsByMerchant(merchantID int, limit, offset int) ([]*SyncedReview, error)
	UpdateSyncedReview(review *SyncedReview) error
	DeleteSyncedReview(id int) error

	// Sync Logs
	CreateSyncLog(log *SyncLog) error
	GetSyncLog(id int) (*SyncLog, error)
	GetSyncLogsByConnection(connectionID int, limit int) ([]*SyncLog, error)
	UpdateSyncLog(log *SyncLog) error

	// Helper methods
	Begin() (*sql.Tx, error)
	Commit(tx *sql.Tx) error
	Rollback(tx *sql.Tx) error
}
