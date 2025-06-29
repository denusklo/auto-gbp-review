package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StorageConfig holds Supabase storage configuration
type StorageConfig struct {
	SupabaseURL        string
	SupabaseServiceKey string
	StorageBucket      string
}

// getStorageConfig initializes storage configuration from environment variables
func getStorageConfig() *StorageConfig {
	return &StorageConfig{
		SupabaseURL:        os.Getenv("SUPABASE_URL"),
		SupabaseServiceKey: os.Getenv("SUPABASE_SERVICE_KEY"),
		StorageBucket:      getEnvWithDefault("STORAGE_BUCKET", "merchant-logos"),
	}
}

// uploadToSupabase uploads a file to Supabase Storage and returns the public URL
func uploadToSupabase(file multipart.File, header *multipart.FileHeader, folder string) (string, error) {
	storageConfig := getStorageConfig()

	// Validate required config
	if storageConfig.SupabaseURL == "" || storageConfig.SupabaseServiceKey == "" {
		return "", fmt.Errorf("Supabase configuration missing. Please check SUPABASE_URL and SUPABASE_SERVICE_KEY")
	}

	// Generate unique filename with timestamp
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".jpg" // default extension
	}

	// Validate file extension
	allowedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	isValidExt := false
	for _, validExt := range allowedExts {
		if ext == validExt {
			isValidExt = true
			break
		}
	}
	if !isValidExt {
		return "", fmt.Errorf("invalid file type. Allowed: jpg, jpeg, png, gif, webp")
	}

	// Create unique filename: folder/timestamp_uuid.ext
	filename := fmt.Sprintf("%s/%d_%s%s", folder, time.Now().Unix(), uuid.New().String()[:8], ext)

	// Read file content
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Check file size (limit to 5MB)
	if len(fileBytes) > 5*1024*1024 {
		return "", fmt.Errorf("file too large. Maximum size is 5MB")
	}

	// Build Supabase Storage API URL
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", storageConfig.SupabaseURL, storageConfig.StorageBucket, filename)

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewReader(fileBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+storageConfig.SupabaseServiceKey)
	req.Header.Set("Content-Type", header.Header.Get("Content-Type"))
	req.Header.Set("Cache-Control", "3600")

	// Make the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Return public URL (for public bucket)
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", storageConfig.SupabaseURL, storageConfig.StorageBucket, filename)
	return publicURL, nil
}
