package socialmedia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GoogleBusinessProvider implements SocialMediaProvider for Google Business Profile
type GoogleBusinessProvider struct {
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
}

// NewGoogleBusinessProvider creates a new Google Business Profile provider
func NewGoogleBusinessProvider(clientID, clientSecret, redirectURI string) *GoogleBusinessProvider {
	return &GoogleBusinessProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// GetPlatformName returns the platform identifier
func (p *GoogleBusinessProvider) GetPlatformName() string {
	return PlatformGoogleBusiness
}

// GetAuthorizationURL returns the OAuth authorization URL
func (p *GoogleBusinessProvider) GetAuthorizationURL(state string) string {
	baseURL := "https://accounts.google.com/o/oauth2/v2/auth"
	params := url.Values{}
	params.Add("client_id", p.clientID)
	params.Add("redirect_uri", p.redirectURI)
	params.Add("response_type", "code")
	params.Add("scope", "https://www.googleapis.com/auth/business.manage")
	params.Add("access_type", "offline")
	params.Add("prompt", "consent")
	params.Add("state", state)

	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

// ExchangeCodeForToken exchanges an authorization code for access and refresh tokens
func (p *GoogleBusinessProvider) ExchangeCodeForToken(code string) (*TokenResponse, error) {
	// Debug logging
	fmt.Printf("Google ExchangeCodeForToken - code: %s\n", code[:20]+"...")
	fmt.Printf("Google ExchangeCodeForToken - redirectURI: %s\n", p.redirectURI)

	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", p.clientID)
	data.Set("client_secret", p.clientSecret)
	data.Set("redirect_uri", p.redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		TokenType:    result.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

// RefreshToken uses a refresh token to get a new access token
func (p *GoogleBusinessProvider) RefreshToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", p.clientID)
	data.Set("client_secret", p.clientSecret)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   result.ExpiresIn,
		TokenType:   result.TokenType,
		ExpiresAt:   time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

// ValidateToken checks if an access token is still valid
func (p *GoogleBusinessProvider) ValidateToken(accessToken string) (bool, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v1/tokeninfo", nil)
	if err != nil {
		return false, err
	}

	q := req.URL.Query()
	q.Add("access_token", accessToken)
	req.URL.RawQuery = q.Encode()

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetAccountInfo retrieves account information
func (p *GoogleBusinessProvider) GetAccountInfo(accessToken string) (*AccountInfo, error) {
	// First, get the list of accounts
	req, err := http.NewRequest("GET", "https://mybusinessaccountmanagement.googleapis.com/v1/accounts", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get accounts: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Accounts []struct {
			Name        string `json:"name"`
			AccountName string `json:"accountName"`
			Type        string `json:"type"`
		} `json:"accounts"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Accounts) == 0 {
		return nil, fmt.Errorf("no business accounts found")
	}

	// Return the first account (in a real implementation, you might want to let the user choose)
	account := result.Accounts[0]
	accountID := account.Name
	if idx := strings.LastIndex(accountID, "/"); idx != -1 {
		accountID = accountID[idx+1:]
	}

	return &AccountInfo{
		AccountID:   accountID,
		AccountName: account.AccountName,
	}, nil
}

// FetchReviews fetches reviews from Google Business Profile
func (p *GoogleBusinessProvider) FetchReviews(accessToken string, since time.Time) ([]*Review, error) {
	// First get the account
	accountInfo, err := p.GetAccountInfo(accessToken)
	if err != nil {
		return nil, err
	}

	// Get list of locations for this account
	locationsURL := fmt.Sprintf("https://mybusinessbusinessinformation.googleapis.com/v1/accounts/%s/locations", accountInfo.AccountID)
	req, err := http.NewRequest("GET", locationsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get locations: %s - %s", resp.Status, string(body))
	}

	var locationsResult struct {
		Locations []struct {
			Name string `json:"name"`
		} `json:"locations"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&locationsResult); err != nil {
		return nil, err
	}

	if len(locationsResult.Locations) == 0 {
		return []*Review{}, nil
	}

	// Fetch reviews for each location
	var allReviews []*Review

	for _, location := range locationsResult.Locations {
		reviewsURL := fmt.Sprintf("https://mybusiness.googleapis.com/v4/%s/reviews", location.Name)
		req, err := http.NewRequest("GET", reviewsURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := p.httpClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var reviewsResult struct {
			Reviews []struct {
				ReviewID   string `json:"reviewId"`
				Reviewer   struct {
					DisplayName string `json:"displayName"`
					ProfilePhotoURL string `json:"profilePhotoUrl"`
				} `json:"reviewer"`
				StarRating string `json:"starRating"` // "ONE", "TWO", "THREE", "FOUR", "FIVE"
				Comment    string `json:"comment"`
				CreateTime string `json:"createTime"`
				UpdateTime string `json:"updateTime"`
				ReviewReply struct {
					Comment    string `json:"comment"`
					UpdateTime string `json:"updateTime"`
				} `json:"reviewReply"`
			} `json:"reviews"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&reviewsResult); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Convert to normalized Review format
		for _, gbpReview := range reviewsResult.Reviews {
			reviewTime, _ := time.Parse(time.RFC3339, gbpReview.CreateTime)

			// Skip if before "since" time
			if !since.IsZero() && reviewTime.Before(since) {
				continue
			}

			// Convert star rating
			rating := p.convertStarRating(gbpReview.StarRating)

			review := &Review{
				PlatformReviewID: gbpReview.ReviewID,
				AuthorName:       gbpReview.Reviewer.DisplayName,
				AuthorPhotoURL:   gbpReview.Reviewer.ProfilePhotoURL,
				Rating:           &rating,
				ReviewText:       gbpReview.Comment,
				ReviewReply:      gbpReview.ReviewReply.Comment,
				ReviewedAt:       reviewTime,
				Metadata: map[string]interface{}{
					"location_name": location.Name,
					"update_time":   gbpReview.UpdateTime,
				},
			}

			allReviews = append(allReviews, review)
		}
	}

	return allReviews, nil
}

// convertStarRating converts Google's star rating string to numeric value
func (p *GoogleBusinessProvider) convertStarRating(starRating string) float64 {
	switch starRating {
	case "ONE":
		return 1.0
	case "TWO":
		return 2.0
	case "THREE":
		return 3.0
	case "FOUR":
		return 4.0
	case "FIVE":
		return 5.0
	default:
		return 0.0
	}
}
