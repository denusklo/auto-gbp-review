package main

import (
	"fmt"
	"os"

	supa "github.com/nedpals/supabase-go"
)

var supabaseClient *supa.Client

// InitSupabase initializes the Supabase client
func InitSupabase() error {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseAnonKey := os.Getenv("SUPABASE_ANON_KEY")
	
	if supabaseURL == "" || supabaseAnonKey == "" {
		return fmt.Errorf("SUPABASE_URL and SUPABASE_ANON_KEY are required")
	}

	client := supa.CreateClient(supabaseURL, supabaseAnonKey)

	supabaseClient = client
	return nil
}

// GetSupabaseClient returns the initialized Supabase client
func GetSupabaseClient() *supa.Client {
	return supabaseClient
}