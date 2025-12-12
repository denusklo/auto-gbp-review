package socialmedia

import (
	"time"
)

// SocialMediaProvider defines the interface that all social media platform integrations must implement
type SocialMediaProvider interface {
	// GetAuthorizationURL returns the OAuth authorization URL with the given state parameter
	GetAuthorizationURL(state string) string

	// ExchangeCodeForToken exchanges an authorization code for access and refresh tokens
	ExchangeCodeForToken(code string) (*TokenResponse, error)

	// RefreshToken uses a refresh token to get a new access token
	RefreshToken(refreshToken string) (*TokenResponse, error)

	// FetchReviews fetches reviews from the platform since the given time
	// If since is zero, fetches all available reviews
	FetchReviews(accessToken string, since time.Time) ([]*Review, error)

	// GetAccountInfo retrieves account information using the access token
	GetAccountInfo(accessToken string) (*AccountInfo, error)

	// GetPlatformName returns the platform identifier
	GetPlatformName() string

	// ValidateToken checks if an access token is still valid
	ValidateToken(accessToken string) (bool, error)
}

// SyncService handles the synchronization of reviews from social media platforms
type SyncService struct {
	db        SocialMediaDB
	providers map[string]SocialMediaProvider
	encryptor TokenEncryptor
}

// NewSyncService creates a new sync service
func NewSyncService(db SocialMediaDB, encryptor TokenEncryptor) *SyncService {
	return &SyncService{
		db:        db,
		providers: make(map[string]SocialMediaProvider),
		encryptor: encryptor,
	}
}

// RegisterProvider registers a social media provider
func (s *SyncService) RegisterProvider(provider SocialMediaProvider) {
	s.providers[provider.GetPlatformName()] = provider
}

// GetProvider returns a provider by platform name
func (s *SyncService) GetProvider(platform string) (SocialMediaProvider, bool) {
	provider, ok := s.providers[platform]
	return provider, ok
}

// SyncConnection syncs reviews for a specific API connection
func (s *SyncService) SyncConnection(connectionID int, syncType string) (*SyncStats, error) {
	// Get the API connection
	conn, err := s.db.GetAPIConnection(connectionID)
	if err != nil {
		return nil, err
	}

	// Get the provider
	provider, ok := s.GetProvider(conn.Platform)
	if !ok {
		return nil, &ErrProviderNotFound{Platform: conn.Platform}
	}

	// Create sync log
	log := &SyncLog{
		APIConnectionID: connectionID,
		SyncType:        syncType,
		Status:          "started",
		StartedAt:       time.Now(),
	}
	if err := s.db.CreateSyncLog(log); err != nil {
		return nil, err
	}

	// Update connection status
	conn.SyncStatus = SyncStatusSyncing
	if err := s.db.UpdateAPIConnection(conn); err != nil {
		return nil, err
	}

	// Decrypt access token
	accessToken, err := s.encryptor.Decrypt(conn.AccessToken)
	if err != nil {
		s.handleSyncError(conn, log, err)
		return nil, err
	}

	// Check if token is valid, refresh if needed
	valid, err := provider.ValidateToken(accessToken)
	if err != nil || !valid {
		if conn.RefreshToken != "" {
			refreshToken, _ := s.encryptor.Decrypt(conn.RefreshToken)
			tokenResp, err := provider.RefreshToken(refreshToken)
			if err != nil {
				s.handleSyncError(conn, log, err)
				return nil, err
			}
			accessToken = tokenResp.AccessToken

			// Update stored tokens
			encryptedAccess, _ := s.encryptor.Encrypt(tokenResp.AccessToken)
			conn.AccessToken = encryptedAccess
			if tokenResp.RefreshToken != "" {
				encryptedRefresh, _ := s.encryptor.Encrypt(tokenResp.RefreshToken)
				conn.RefreshToken = encryptedRefresh
			}
			conn.TokenExpiresAt = tokenResp.ExpiresAt
			s.db.UpdateAPIConnection(conn)
		} else {
			s.handleSyncError(conn, log, &ErrInvalidToken{})
			return nil, &ErrInvalidToken{}
		}
	}

	// Fetch reviews since last sync
	since := time.Time{}
	if conn.LastSyncAt != nil {
		since = *conn.LastSyncAt
	}

	reviews, err := provider.FetchReviews(accessToken, since)
	if err != nil {
		s.handleSyncError(conn, log, err)
		return nil, err
	}

	// Process reviews
	stats := &SyncStats{
		TotalFetched: len(reviews),
	}

	for _, review := range reviews {
		// Check if review already exists
		existing, err := s.db.GetSyncedReviewByPlatformID(conn.Platform, review.PlatformReviewID)

		syncedReview := &SyncedReview{
			MerchantID:       conn.MerchantID,
			APIConnectionID:  &conn.ID,
			Platform:         conn.Platform,
			PlatformReviewID: review.PlatformReviewID,
			AuthorName:       review.AuthorName,
			AuthorPhotoURL:   review.AuthorPhotoURL,
			Rating:           review.Rating,
			ReviewText:       review.ReviewText,
			ReviewReply:      review.ReviewReply,
			ReviewedAt:       review.ReviewedAt,
			IsVisible:        true,
			Metadata:         review.Metadata,
		}

		if err != nil || existing == nil {
			// Create new review
			if err := s.db.CreateSyncedReview(syncedReview); err != nil {
				stats.Errors = append(stats.Errors, err)
			} else {
				stats.TotalAdded++
			}
		} else {
			// Update existing review
			syncedReview.ID = existing.ID
			if err := s.db.UpdateSyncedReview(syncedReview); err != nil {
				stats.Errors = append(stats.Errors, err)
			} else {
				stats.TotalUpdated++
			}
		}
	}

	// Update connection
	now := time.Now()
	conn.LastSyncAt = &now
	conn.SyncStatus = SyncStatusCompleted
	conn.ErrorMessage = ""
	if err := s.db.UpdateAPIConnection(conn); err != nil {
		return stats, err
	}

	// Complete sync log
	log.Status = "completed"
	log.ReviewsFetched = stats.TotalFetched
	log.ReviewsAdded = stats.TotalAdded
	log.ReviewsUpdated = stats.TotalUpdated
	log.CompletedAt = &now
	s.db.UpdateSyncLog(log)

	return stats, nil
}

// handleSyncError handles sync errors by updating connection and log
func (s *SyncService) handleSyncError(conn *APIConnection, log *SyncLog, err error) {
	conn.SyncStatus = SyncStatusFailed
	conn.ErrorMessage = err.Error()
	s.db.UpdateAPIConnection(conn)

	now := time.Now()
	log.Status = "failed"
	log.ErrorMessage = err.Error()
	log.CompletedAt = &now
	s.db.UpdateSyncLog(log)
}

// SyncAllActiveConnections syncs all active connections
func (s *SyncService) SyncAllActiveConnections() error {
	connections, err := s.db.GetActiveConnections()
	if err != nil {
		return err
	}

	for _, conn := range connections {
		// Skip if already syncing
		if conn.SyncStatus == SyncStatusSyncing {
			continue
		}

		// Sync in background (could use goroutines with proper error handling)
		_, _ = s.SyncConnection(conn.ID, SyncTypeScheduled)
	}

	return nil
}

// TokenEncryptor interface for encrypting/decrypting tokens
type TokenEncryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Custom errors
type ErrProviderNotFound struct {
	Platform string
}

func (e *ErrProviderNotFound) Error() string {
	return "provider not found for platform: " + e.Platform
}

type ErrInvalidToken struct{}

func (e *ErrInvalidToken) Error() string {
	return "invalid or expired access token"
}
