package socialmedia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// FacebookProvider implements SocialMediaProvider for Facebook Page Reviews
type FacebookProvider struct {
	appID       string
	appSecret   string
	redirectURI string
	httpClient  *http.Client
}

// NewFacebookProvider creates a new Facebook provider
func NewFacebookProvider(appID, appSecret, redirectURI string) *FacebookProvider {
	return &FacebookProvider{
		appID:       appID,
		appSecret:   appSecret,
		redirectURI: redirectURI,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// GetPlatformName returns the platform identifier
func (p *FacebookProvider) GetPlatformName() string {
	return PlatformFacebook
}

// GetAuthorizationURL returns the OAuth authorization URL
func (p *FacebookProvider) GetAuthorizationURL(state string) string {
	baseURL := "https://www.facebook.com/v18.0/dialog/oauth"
	params := url.Values{}
	params.Add("client_id", p.appID)
	params.Add("redirect_uri", p.redirectURI)
	params.Add("state", state)
	params.Add("scope", "pages_show_list,pages_read_engagement,pages_manage_metadata")

	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

// ExchangeCodeForToken exchanges an authorization code for access token
func (p *FacebookProvider) ExchangeCodeForToken(code string) (*TokenResponse, error) {
	tokenURL := "https://graph.facebook.com/v18.0/oauth/access_token"
	params := url.Values{}
	params.Add("client_id", p.appID)
	params.Add("client_secret", p.appSecret)
	params.Add("redirect_uri", p.redirectURI)
	params.Add("code", code)

	resp, err := p.httpClient.Get(fmt.Sprintf("%s?%s", tokenURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Get long-lived token
	longLivedToken, err := p.getLongLivedToken(result.AccessToken)
	if err != nil {
		// If we can't get long-lived token, use the short-lived one
		longLivedToken = &result
	}

	return &TokenResponse{
		AccessToken: longLivedToken.AccessToken,
		ExpiresIn:   longLivedToken.ExpiresIn,
		TokenType:   longLivedToken.TokenType,
		ExpiresAt:   time.Now().Add(time.Duration(longLivedToken.ExpiresIn) * time.Second),
	}, nil
}

// getLongLivedToken exchanges a short-lived token for a long-lived one
func (p *FacebookProvider) getLongLivedToken(shortLivedToken string) (*struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}, error) {
	tokenURL := "https://graph.facebook.com/v18.0/oauth/access_token"
	params := url.Values{}
	params.Add("grant_type", "fb_exchange_token")
	params.Add("client_id", p.appID)
	params.Add("client_secret", p.appSecret)
	params.Add("fb_exchange_token", shortLivedToken)

	resp, err := p.httpClient.Get(fmt.Sprintf("%s?%s", tokenURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("long-lived token exchange failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// RefreshToken - Facebook doesn't support refresh tokens, but we can try to extend the token
func (p *FacebookProvider) RefreshToken(refreshToken string) (*TokenResponse, error) {
	// For Facebook, we try to get a long-lived token again
	longLivedToken, err := p.getLongLivedToken(refreshToken)
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken: longLivedToken.AccessToken,
		ExpiresIn:   longLivedToken.ExpiresIn,
		TokenType:   longLivedToken.TokenType,
		ExpiresAt:   time.Now().Add(time.Duration(longLivedToken.ExpiresIn) * time.Second),
	}, nil
}

// ValidateToken checks if an access token is still valid
func (p *FacebookProvider) ValidateToken(accessToken string) (bool, error) {
	debugURL := fmt.Sprintf("https://graph.facebook.com/v18.0/debug_token?input_token=%s&access_token=%s|%s",
		accessToken, p.appID, p.appSecret)

	resp, err := p.httpClient.Get(debugURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result struct {
		Data struct {
			IsValid   bool  `json:"is_valid"`
			ExpiresAt int64 `json:"expires_at"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	return result.Data.IsValid && result.Data.ExpiresAt > time.Now().Unix(), nil
}

// GetAccountInfo retrieves Facebook Page information
func (p *FacebookProvider) GetAccountInfo(accessToken string) (*AccountInfo, error) {
	// Get user's pages
	pagesURL := fmt.Sprintf("https://graph.facebook.com/v18.0/me/accounts?access_token=%s", accessToken)

	resp, err := p.httpClient.Get(pagesURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get pages: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no Facebook pages found")
	}

	// Return the first page (in a real implementation, let user choose)
	page := result.Data[0]

	return &AccountInfo{
		AccountID:   page.ID,
		AccountName: page.Name,
	}, nil
}

// FetchReviews fetches reviews from Facebook Page
func (p *FacebookProvider) FetchReviews(accessToken string, since time.Time) ([]*Review, error) {
	// Get account info to get page ID
	accountInfo, err := p.GetAccountInfo(accessToken)
	if err != nil {
		return nil, err
	}

	// Get page access token
	pageToken, err := p.getPageAccessToken(accessToken, accountInfo.AccountID)
	if err != nil {
		return nil, err
	}

	// Fetch ratings and reviews
	reviewsURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/ratings?fields=reviewer,created_time,rating,review_text,recommendation_type,open_graph_story&access_token=%s",
		accountInfo.AccountID, pageToken)

	// Add since parameter if provided
	if !since.IsZero() {
		reviewsURL += fmt.Sprintf("&since=%d", since.Unix())
	}

	resp, err := p.httpClient.Get(reviewsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch reviews: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Data []struct {
			CreatedTime string `json:"created_time"`
			Reviewer    struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"reviewer"`
			Rating              int    `json:"rating"`
			ReviewText          string `json:"review_text"`
			RecommendationType  string `json:"recommendation_type"`
			OpenGraphStory      *struct {
				ID string `json:"id"`
			} `json:"open_graph_story"`
		} `json:"data"`
		Paging struct {
			Cursors struct {
				Before string `json:"before"`
				After  string `json:"after"`
			} `json:"cursors"`
			Next string `json:"next"`
		} `json:"paging"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Convert to normalized Review format
	var reviews []*Review

	for _, fbReview := range result.Data {
		reviewTime, _ := time.Parse(time.RFC3339, fbReview.CreatedTime)

		// Use open graph story ID as review ID, fallback to created_time
		reviewID := fbReview.CreatedTime
		if fbReview.OpenGraphStory != nil {
			reviewID = fbReview.OpenGraphStory.ID
		}

		rating := float64(fbReview.Rating)

		review := &Review{
			PlatformReviewID: reviewID,
			AuthorName:       fbReview.Reviewer.Name,
			Rating:           &rating,
			ReviewText:       fbReview.ReviewText,
			ReviewedAt:       reviewTime,
			Metadata: map[string]interface{}{
				"reviewer_id":         fbReview.Reviewer.ID,
				"recommendation_type": fbReview.RecommendationType,
				"page_id":             accountInfo.AccountID,
			},
		}

		reviews = append(reviews, review)
	}

	// TODO: Handle pagination if there are more reviews
	// For now, just return the first page of results

	return reviews, nil
}

// getPageAccessToken gets the page access token for a specific page
func (p *FacebookProvider) getPageAccessToken(userAccessToken, pageID string) (string, error) {
	pagesURL := fmt.Sprintf("https://graph.facebook.com/v18.0/me/accounts?access_token=%s", userAccessToken)

	resp, err := p.httpClient.Get(pagesURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get page token: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, page := range result.Data {
		if page.ID == pageID {
			return page.AccessToken, nil
		}
	}

	return "", fmt.Errorf("page access token not found for page ID: %s", pageID)
}
