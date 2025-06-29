package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// Add this struct definition
type PlacesAPIResponse struct {
	Results []struct {
		PlaceID string `json:"place_id"`
		Name    string `json:"name"`
	} `json:"results"`
	Status string `json:"status"`
}

// GenerateWhatsAppWebLink creates a WhatsApp Web link
func GenerateWhatsAppWebLink(phoneNumber, message string) string {
	// Clean phone number (remove + for web version)
	cleanPhone := strings.ReplaceAll(phoneNumber, "+", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, " ", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, "(", "")
	cleanPhone = strings.ReplaceAll(cleanPhone, ")", "")

	return fmt.Sprintf(
		"https://web.whatsapp.com/send?phone=%s&text=%s",
		cleanPhone,
		url.QueryEscape(message),
	)
}

// GenerateWhatsAppAppLink creates a WhatsApp app link
func GenerateWhatsAppAppLink(phoneNumber, message string) string {
	return fmt.Sprintf(
		"https://api.whatsapp.com/send/?phone=%s&text=%s&type=phone_number&app_absent=0",
		phoneNumber, // Keep the + for API version
		url.QueryEscape(message),
	)
}

// In your Go backend
func GetGooglePlaceID(businessName, address string) (string, error) {
	apiKey := os.Getenv("GOOGLE_PLACES_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GOOGLE_PLACES_API_KEY not set")
	}

	// Combine business name and address for better search results
	query := fmt.Sprintf("%s %s", businessName, address)

	// Create the API URL
	apiURL := fmt.Sprintf(
		"https://maps.googleapis.com/maps/api/place/textsearch/json?query=%s&key=%s",
		url.QueryEscape(query),
		apiKey,
	)

	// Make the API request
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to make API request: %v", err)
	}
	defer resp.Body.Close()

	// Parse the response
	var result PlacesAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode API response: %v", err)
	}

	// Check if we got results
	if result.Status != "OK" {
		return "", fmt.Errorf("API returned status: %s", result.Status)
	}

	if len(result.Results) == 0 {
		return "", fmt.Errorf("no places found for query: %s", query)
	}

	// Return the first result's Place ID
	return result.Results[0].PlaceID, nil
}

// GenerateWazeURL creates a Waze URL similar to the example format
func GenerateWazeURL(businessName, address, placeID string) string {
	if placeID == "" {
		// Fallback to simple search
		return fmt.Sprintf("https://waze.com/ul?q=%s&navigate=yes", url.QueryEscape(address))
	}

	// Create business slug
	businessSlug := strings.ToLower(businessName)
	businessSlug = regexp.MustCompile(`[^a-z0-9\s]`).ReplaceAllString(businessSlug, "")
	businessSlug = regexp.MustCompile(`\s+`).ReplaceAllString(businessSlug, "-")
	businessSlug = strings.Trim(businessSlug, "-")

	// Parse location from address
	state, city := parseLocationFromAddress(address)

	return fmt.Sprintf(
		"https://www.waze.com/live-map/directions/my/%s/%s/%s?navigate=yes&utm_campaign=default&utm_source=waze_website&utm_medium=lm_share_location&to=place.%s",
		state, city, businessSlug, placeID,
	)
}

func parseLocationFromAddress(address string) (state, city string) {
	// Default values
	state = "johor-darul-tazim"
	city = "johor-bahru"

	if address == "" {
		return state, city
	}

	addressLower := strings.ToLower(address)

	// Malaysian states mapping
	stateMap := map[string]string{
		"johor":           "johor-darul-tazim",
		"kuala lumpur":    "kuala-lumpur",
		"selangor":        "selangor",
		"penang":          "penang",
		"perak":           "perak",
		"kedah":           "kedah",
		"kelantan":        "kelantan",
		"terengganu":      "terengganu",
		"pahang":          "pahang",
		"negeri sembilan": "negeri-sembilan",
		"melaka":          "melaka",
		"sabah":           "sabah",
		"sarawak":         "sarawak",
	}

	// City mapping (simplified)
	cityMap := map[string]string{
		"johor bahru":   "johor-bahru",
		"kuala lumpur":  "kuala-lumpur",
		"petaling jaya": "petaling-jaya",
		"shah alam":     "shah-alam",
		"george town":   "george-town",
		"ipoh":          "ipoh",
		"kuching":       "kuching",
		"kota kinabalu": "kota-kinabalu",
	}

	// Check for states
	for stateName, stateSlug := range stateMap {
		if strings.Contains(addressLower, stateName) {
			state = stateSlug
			break
		}
	}

	// Check for cities
	for cityName, citySlug := range cityMap {
		if strings.Contains(addressLower, cityName) {
			city = citySlug
			break
		}
	}

	return state, city
}
