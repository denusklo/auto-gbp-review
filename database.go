package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

type Database struct {
	*sql.DB
}

func InitDatabase() (*Database, error) {
	var connStr string

	// Check if using Supabase
	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL != "" {
		projectID := extractProjectID(supabaseURL)
		password := os.Getenv("SUPABASE_DB_PASSWORD")
		if password == "" {
			return nil, fmt.Errorf("SUPABASE_DB_PASSWORD environment variable is required")
		}

		// Use Session Pooler (IPv4 compatible)
		connStr = fmt.Sprintf("postgresql://postgres.%s:%s@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=require",
			projectID, password)
	} else {
		// Use local PostgreSQL
		connStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			getEnvWithDefault("DB_HOST", "localhost"),
			getEnvWithDefault("DB_PORT", "5432"),
			getEnvWithDefault("DB_USER", "postgres"),
			getEnvWithDefault("DB_PASSWORD", "postgres"),
			getEnvWithDefault("DB_NAME", "auto_gbp_review"))
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	database := &Database{db}

	// Run migrations
	if err := database.migrate(); err != nil {
		return nil, err
	}

	return database, nil
}

// migrate runs database migrations
func (db *Database) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(50) DEFAULT 'merchant',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS merchants (
			id SERIAL PRIMARY KEY,
			auth_user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
			business_name VARCHAR(255) NOT NULL,
			slug VARCHAR(255) UNIQUE NOT NULL,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS merchant_details (
			id SERIAL PRIMARY KEY,
			merchant_id INTEGER REFERENCES merchants(id) ON DELETE CASCADE,
			address TEXT,
			phone_number VARCHAR(50),
			whatsapp_preset_text TEXT DEFAULT 'I''m interested in your services',
			facebook_url VARCHAR(500),
			xiaohongshu_id VARCHAR(255),
			tiktok_url VARCHAR(500),
			instagram_url VARCHAR(500),
			threads_url VARCHAR(500),
			website_url VARCHAR(500),
			google_play_url VARCHAR(500),
			app_store_url VARCHAR(500),
			google_maps_url VARCHAR(500),
			waze_url VARCHAR(500),
			logo_url VARCHAR(500),
			theme_color VARCHAR(7) DEFAULT '#3B82F6',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_merchants_slug ON merchants(slug)`,
		`CREATE INDEX IF NOT EXISTS idx_merchants_auth_user_id ON merchants(auth_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_merchant_details_merchant_id ON merchant_details(merchant_id)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %v", err)
		}
	}

	return nil
}

// Helper functions
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func extractProjectID(supabaseURL string) string {
	// Extract project ID from https://your-project.supabase.co
	// Remove the protocol and split by dots
	url := strings.TrimPrefix(supabaseURL, "https://")
	url = strings.TrimPrefix(url, "http://")
	parts := strings.Split(url, ".")
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}
