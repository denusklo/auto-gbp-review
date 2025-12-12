package socialmedia

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// DB wraps a sql.DB to implement SocialMediaDB interface
type DB struct {
	conn *sql.DB
}

// NewDB creates a new social media database wrapper
func NewDB(conn *sql.DB) *DB {
	return &DB{conn: conn}
}

// API Connections

func (db *DB) CreateAPIConnection(conn *APIConnection) error {
	query := `
		INSERT INTO api_connections (
			merchant_id, platform, platform_account_id, platform_account_name,
			access_token, refresh_token, token_expires_at, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`
	return db.conn.QueryRow(
		query,
		conn.MerchantID, conn.Platform, conn.PlatformAccountID, conn.PlatformAccountName,
		conn.AccessToken, conn.RefreshToken, conn.TokenExpiresAt, conn.IsActive,
	).Scan(&conn.ID, &conn.CreatedAt, &conn.UpdatedAt)
}

func (db *DB) GetAPIConnection(id int) (*APIConnection, error) {
	conn := &APIConnection{}
	var lastSyncAt sql.NullTime

	query := `
		SELECT id, merchant_id, platform, platform_account_id, platform_account_name,
			access_token, refresh_token, token_expires_at, is_active, last_sync_at,
			sync_status, error_message, created_at, updated_at
		FROM api_connections
		WHERE id = $1
	`
	err := db.conn.QueryRow(query, id).Scan(
		&conn.ID, &conn.MerchantID, &conn.Platform, &conn.PlatformAccountID, &conn.PlatformAccountName,
		&conn.AccessToken, &conn.RefreshToken, &conn.TokenExpiresAt, &conn.IsActive, &lastSyncAt,
		&conn.SyncStatus, &conn.ErrorMessage, &conn.CreatedAt, &conn.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if lastSyncAt.Valid {
		conn.LastSyncAt = &lastSyncAt.Time
	}

	return conn, nil
}

func (db *DB) GetAPIConnectionsByMerchant(merchantID int) ([]*APIConnection, error) {
	query := `
		SELECT id, merchant_id, platform, platform_account_id, platform_account_name,
			access_token, refresh_token, token_expires_at, is_active, last_sync_at,
			sync_status, error_message, created_at, updated_at
		FROM api_connections
		WHERE merchant_id = $1
		ORDER BY created_at DESC
	`
	rows, err := db.conn.Query(query, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []*APIConnection
	for rows.Next() {
		conn := &APIConnection{}
		var lastSyncAt sql.NullTime

		err := rows.Scan(
			&conn.ID, &conn.MerchantID, &conn.Platform, &conn.PlatformAccountID, &conn.PlatformAccountName,
			&conn.AccessToken, &conn.RefreshToken, &conn.TokenExpiresAt, &conn.IsActive, &lastSyncAt,
			&conn.SyncStatus, &conn.ErrorMessage, &conn.CreatedAt, &conn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if lastSyncAt.Valid {
			conn.LastSyncAt = &lastSyncAt.Time
		}

		connections = append(connections, conn)
	}

	return connections, nil
}

func (db *DB) GetAPIConnectionByPlatform(merchantID int, platform string) (*APIConnection, error) {
	conn := &APIConnection{}
	var lastSyncAt sql.NullTime

	query := `
		SELECT id, merchant_id, platform, platform_account_id, platform_account_name,
			access_token, refresh_token, token_expires_at, is_active, last_sync_at,
			sync_status, error_message, created_at, updated_at
		FROM api_connections
		WHERE merchant_id = $1 AND platform = $2
		LIMIT 1
	`
	err := db.conn.QueryRow(query, merchantID, platform).Scan(
		&conn.ID, &conn.MerchantID, &conn.Platform, &conn.PlatformAccountID, &conn.PlatformAccountName,
		&conn.AccessToken, &conn.RefreshToken, &conn.TokenExpiresAt, &conn.IsActive, &lastSyncAt,
		&conn.SyncStatus, &conn.ErrorMessage, &conn.CreatedAt, &conn.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if lastSyncAt.Valid {
		conn.LastSyncAt = &lastSyncAt.Time
	}

	return conn, nil
}

func (db *DB) UpdateAPIConnection(conn *APIConnection) error {
	query := `
		UPDATE api_connections
		SET platform_account_id = $1, platform_account_name = $2, access_token = $3,
			refresh_token = $4, token_expires_at = $5, is_active = $6, last_sync_at = $7,
			sync_status = $8, error_message = $9, updated_at = CURRENT_TIMESTAMP
		WHERE id = $10
	`
	_, err := db.conn.Exec(
		query,
		conn.PlatformAccountID, conn.PlatformAccountName, conn.AccessToken,
		conn.RefreshToken, conn.TokenExpiresAt, conn.IsActive, conn.LastSyncAt,
		conn.SyncStatus, conn.ErrorMessage, conn.ID,
	)
	return err
}

func (db *DB) DeleteAPIConnection(id int) error {
	query := `DELETE FROM api_connections WHERE id = $1`
	_, err := db.conn.Exec(query, id)
	return err
}

func (db *DB) GetActiveConnections() ([]*APIConnection, error) {
	query := `
		SELECT id, merchant_id, platform, platform_account_id, platform_account_name,
			access_token, refresh_token, token_expires_at, is_active, last_sync_at,
			sync_status, error_message, created_at, updated_at
		FROM api_connections
		WHERE is_active = true
		ORDER BY last_sync_at ASC NULLS FIRST
	`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []*APIConnection
	for rows.Next() {
		conn := &APIConnection{}
		var lastSyncAt sql.NullTime

		err := rows.Scan(
			&conn.ID, &conn.MerchantID, &conn.Platform, &conn.PlatformAccountID, &conn.PlatformAccountName,
			&conn.AccessToken, &conn.RefreshToken, &conn.TokenExpiresAt, &conn.IsActive, &lastSyncAt,
			&conn.SyncStatus, &conn.ErrorMessage, &conn.CreatedAt, &conn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if lastSyncAt.Valid {
			conn.LastSyncAt = &lastSyncAt.Time
		}

		connections = append(connections, conn)
	}

	return connections, nil
}

// Synced Reviews

func (db *DB) CreateSyncedReview(review *SyncedReview) error {
	metadataJSON, err := json.Marshal(review.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO synced_reviews (
			merchant_id, api_connection_id, platform, platform_review_id,
			author_name, author_photo_url, rating, review_text, review_reply,
			reviewed_at, is_visible, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, synced_at, created_at, updated_at
	`
	return db.conn.QueryRow(
		query,
		review.MerchantID, review.APIConnectionID, review.Platform, review.PlatformReviewID,
		review.AuthorName, review.AuthorPhotoURL, review.Rating, review.ReviewText, review.ReviewReply,
		review.ReviewedAt, review.IsVisible, metadataJSON,
	).Scan(&review.ID, &review.SyncedAt, &review.CreatedAt, &review.UpdatedAt)
}

func (db *DB) GetSyncedReview(id int) (*SyncedReview, error) {
	review := &SyncedReview{}
	var metadataJSON []byte
	var apiConnectionID sql.NullInt64
	var rating sql.NullFloat64

	query := `
		SELECT id, merchant_id, api_connection_id, platform, platform_review_id,
			author_name, author_photo_url, rating, review_text, review_reply,
			reviewed_at, synced_at, is_visible, metadata, created_at, updated_at
		FROM synced_reviews
		WHERE id = $1
	`
	err := db.conn.QueryRow(query, id).Scan(
		&review.ID, &review.MerchantID, &apiConnectionID, &review.Platform, &review.PlatformReviewID,
		&review.AuthorName, &review.AuthorPhotoURL, &rating, &review.ReviewText, &review.ReviewReply,
		&review.ReviewedAt, &review.SyncedAt, &review.IsVisible, &metadataJSON, &review.CreatedAt, &review.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if apiConnectionID.Valid {
		id := int(apiConnectionID.Int64)
		review.APIConnectionID = &id
	}

	if rating.Valid {
		review.Rating = &rating.Float64
	}

	if len(metadataJSON) > 0 {
		json.Unmarshal(metadataJSON, &review.Metadata)
	}

	return review, nil
}

func (db *DB) GetSyncedReviewByPlatformID(platform, platformReviewID string) (*SyncedReview, error) {
	review := &SyncedReview{}
	var metadataJSON []byte
	var apiConnectionID sql.NullInt64
	var rating sql.NullFloat64

	query := `
		SELECT id, merchant_id, api_connection_id, platform, platform_review_id,
			author_name, author_photo_url, rating, review_text, review_reply,
			reviewed_at, synced_at, is_visible, metadata, created_at, updated_at
		FROM synced_reviews
		WHERE platform = $1 AND platform_review_id = $2
	`
	err := db.conn.QueryRow(query, platform, platformReviewID).Scan(
		&review.ID, &review.MerchantID, &apiConnectionID, &review.Platform, &review.PlatformReviewID,
		&review.AuthorName, &review.AuthorPhotoURL, &rating, &review.ReviewText, &review.ReviewReply,
		&review.ReviewedAt, &review.SyncedAt, &review.IsVisible, &metadataJSON, &review.CreatedAt, &review.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if apiConnectionID.Valid {
		id := int(apiConnectionID.Int64)
		review.APIConnectionID = &id
	}

	if rating.Valid {
		review.Rating = &rating.Float64
	}

	if len(metadataJSON) > 0 {
		json.Unmarshal(metadataJSON, &review.Metadata)
	}

	return review, nil
}

func (db *DB) GetSyncedReviewsByMerchant(merchantID int, limit, offset int) ([]*SyncedReview, error) {
	query := `
		SELECT id, merchant_id, api_connection_id, platform, platform_review_id,
			author_name, author_photo_url, rating, review_text, review_reply,
			reviewed_at, synced_at, is_visible, metadata, created_at, updated_at
		FROM synced_reviews
		WHERE merchant_id = $1 AND is_visible = true
		ORDER BY reviewed_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := db.conn.Query(query, merchantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []*SyncedReview
	for rows.Next() {
		review := &SyncedReview{}
		var metadataJSON []byte
		var apiConnectionID sql.NullInt64
		var rating sql.NullFloat64

		err := rows.Scan(
			&review.ID, &review.MerchantID, &apiConnectionID, &review.Platform, &review.PlatformReviewID,
			&review.AuthorName, &review.AuthorPhotoURL, &rating, &review.ReviewText, &review.ReviewReply,
			&review.ReviewedAt, &review.SyncedAt, &review.IsVisible, &metadataJSON, &review.CreatedAt, &review.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if apiConnectionID.Valid {
			id := int(apiConnectionID.Int64)
			review.APIConnectionID = &id
		}

		if rating.Valid {
			review.Rating = &rating.Float64
		}

		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &review.Metadata)
		}

		reviews = append(reviews, review)
	}

	return reviews, nil
}

func (db *DB) UpdateSyncedReview(review *SyncedReview) error {
	metadataJSON, err := json.Marshal(review.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	query := `
		UPDATE synced_reviews
		SET author_name = $1, author_photo_url = $2, rating = $3, review_text = $4,
			review_reply = $5, is_visible = $6, metadata = $7, updated_at = CURRENT_TIMESTAMP
		WHERE id = $8
	`
	_, err = db.conn.Exec(
		query,
		review.AuthorName, review.AuthorPhotoURL, review.Rating, review.ReviewText,
		review.ReviewReply, review.IsVisible, metadataJSON, review.ID,
	)
	return err
}

func (db *DB) DeleteSyncedReview(id int) error {
	query := `DELETE FROM synced_reviews WHERE id = $1`
	_, err := db.conn.Exec(query, id)
	return err
}

// Sync Logs

func (db *DB) CreateSyncLog(log *SyncLog) error {
	query := `
		INSERT INTO sync_logs (
			api_connection_id, sync_type, status, reviews_fetched,
			reviews_added, reviews_updated, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, started_at
	`
	return db.conn.QueryRow(
		query,
		log.APIConnectionID, log.SyncType, log.Status, log.ReviewsFetched,
		log.ReviewsAdded, log.ReviewsUpdated, log.ErrorMessage,
	).Scan(&log.ID, &log.StartedAt)
}

func (db *DB) GetSyncLog(id int) (*SyncLog, error) {
	log := &SyncLog{}
	var completedAt sql.NullTime

	query := `
		SELECT id, api_connection_id, sync_type, status, reviews_fetched,
			reviews_added, reviews_updated, error_message, started_at, completed_at
		FROM sync_logs
		WHERE id = $1
	`
	err := db.conn.QueryRow(query, id).Scan(
		&log.ID, &log.APIConnectionID, &log.SyncType, &log.Status, &log.ReviewsFetched,
		&log.ReviewsAdded, &log.ReviewsUpdated, &log.ErrorMessage, &log.StartedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		log.CompletedAt = &completedAt.Time
	}

	return log, nil
}

func (db *DB) GetSyncLogsByConnection(connectionID int, limit int) ([]*SyncLog, error) {
	query := `
		SELECT id, api_connection_id, sync_type, status, reviews_fetched,
			reviews_added, reviews_updated, error_message, started_at, completed_at
		FROM sync_logs
		WHERE api_connection_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`
	rows, err := db.conn.Query(query, connectionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*SyncLog
	for rows.Next() {
		log := &SyncLog{}
		var completedAt sql.NullTime

		err := rows.Scan(
			&log.ID, &log.APIConnectionID, &log.SyncType, &log.Status, &log.ReviewsFetched,
			&log.ReviewsAdded, &log.ReviewsUpdated, &log.ErrorMessage, &log.StartedAt, &completedAt,
		)
		if err != nil {
			return nil, err
		}

		if completedAt.Valid {
			log.CompletedAt = &completedAt.Time
		}

		logs = append(logs, log)
	}

	return logs, nil
}

func (db *DB) UpdateSyncLog(log *SyncLog) error {
	query := `
		UPDATE sync_logs
		SET status = $1, reviews_fetched = $2, reviews_added = $3,
			reviews_updated = $4, error_message = $5, completed_at = $6
		WHERE id = $7
	`
	_, err := db.conn.Exec(
		query,
		log.Status, log.ReviewsFetched, log.ReviewsAdded,
		log.ReviewsUpdated, log.ErrorMessage, log.CompletedAt, log.ID,
	)
	return err
}

// Transaction helpers

func (db *DB) Begin() (*sql.Tx, error) {
	return db.conn.Begin()
}

func (db *DB) Commit(tx *sql.Tx) error {
	return tx.Commit()
}

func (db *DB) Rollback(tx *sql.Tx) error {
	return tx.Rollback()
}

// Stats query helper
func (db *DB) GetMerchantReviewStats(merchantID int) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_reviews,
			COUNT(DISTINCT platform) as platforms_connected,
			AVG(CASE WHEN rating IS NOT NULL THEN rating ELSE 0 END) as avg_rating,
			MAX(reviewed_at) as latest_review_date
		FROM synced_reviews
		WHERE merchant_id = $1 AND is_visible = true
	`

	var totalReviews, platformsConnected int
	var avgRating float64
	var latestReviewDate sql.NullTime

	err := db.conn.QueryRow(query, merchantID).Scan(
		&totalReviews, &platformsConnected, &avgRating, &latestReviewDate,
	)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"total_reviews":       totalReviews,
		"platforms_connected": platformsConnected,
		"avg_rating":          fmt.Sprintf("%.1f", avgRating),
	}

	if latestReviewDate.Valid {
		stats["latest_review_date"] = latestReviewDate.Time
	}

	return stats, nil
}
