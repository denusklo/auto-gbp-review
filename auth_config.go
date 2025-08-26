package main

import (
	"os"
	"strings"
)

// AuthMode determines which authentication system to use
type AuthMode string

const (
	AuthModeJWT      AuthMode = "jwt"      // Legacy JWT auth
	AuthModeSupabase AuthMode = "supabase" // New Supabase Auth
	AuthModeDual     AuthMode = "dual"     // Support both during migration
)

// GetAuthMode returns the current authentication mode
func GetAuthMode() AuthMode {
	mode := strings.ToLower(os.Getenv("AUTH_MODE"))
	switch mode {
	case "supabase":
		return AuthModeSupabase
	case "jwt":
		return AuthModeJWT
	case "dual":
		return AuthModeDual
	default:
		// Default to dual mode for safe migration
		return AuthModeDual
	}
}

// UseSupabaseAuth checks if Supabase Auth should be used
func UseSupabaseAuth() bool {
	mode := GetAuthMode()
	return mode == AuthModeSupabase || mode == AuthModeDual
}

// UseLegacyAuth checks if legacy JWT auth should be used
func UseLegacyAuth() bool {
	mode := GetAuthMode()
	return mode == AuthModeJWT || mode == AuthModeDual
}