package socialmedia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// InstagramProvider implements SocialMediaProvider for Instagram mentions
// Note: Instagram uses the Facebook Graph API
type InstagramProvider struct {
	appID       string
	appSecret   string
	redirectURI string
	httpClient  *http.Client
}

// NewInstagramProvider creates a new Instagram provider
func NewInstagramProvider(appID, appSecret, redirectURI string) *InstagramProvider {
	return &InstagramProvider{
		appID:       appID,
		appSecret:   appSecret,
		redirectURI: redirectURI,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// GetPlatformName returns the platform identifier
func (p *InstagramProvider) GetPlatformName() string {
	return PlatformInstagram
}

// GetAuthorizationURL returns the OAuth authorization URL
func (p *InstagramProvider) GetAuthorizationURL(state string) string {
	baseURL := "https://www.facebook.com/v18.0/dialog/oauth"
	params := url.Values{}
	params.Add("client_id", p.appID)
	params.Add("redirect_uri", p.redirectURI)
	params.Add("state", state)
	// Request Instagram-specific permissions
	params.Add("scope", "instagram_basic,instagram_manage_comments,instagram_manage_insights,pages_show_list")

	return fmt.Sprintf("%s?%s", baseURL, params.Encode())
}

// ExchangeCodeForToken exchanges an authorization code for access token
func (p *InstagramProvider) ExchangeCodeForToken(code string) (*TokenResponse, error) {
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
func (p *InstagramProvider) getLongLivedToken(shortLivedToken string) (*struct {
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

// RefreshToken refreshes the access token
func (p *InstagramProvider) RefreshToken(refreshToken string) (*TokenResponse, error) {
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
func (p *InstagramProvider) ValidateToken(accessToken string) (bool, error) {
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

// GetAccountInfo retrieves Instagram Business Account information
func (p *InstagramProvider) GetAccountInfo(accessToken string) (*AccountInfo, error) {
	// Get user's pages first
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

	var pagesResult struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pagesResult); err != nil {
		return nil, err
	}

	if len(pagesResult.Data) == 0 {
		return nil, fmt.Errorf("no Facebook pages found")
	}

	// Get Instagram Business Account connected to the page
	pageID := pagesResult.Data[0].ID
	pageToken := pagesResult.Data[0].AccessToken

	igAccountURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s?fields=instagram_business_account&access_token=%s",
		pageID, pageToken)

	resp2, err := p.httpClient.Get(igAccountURL)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		return nil, fmt.Errorf("failed to get Instagram account: %s - %s", resp2.Status, string(body))
	}

	var igResult struct {
		InstagramBusinessAccount struct {
			ID string `json:"id"`
		} `json:"instagram_business_account"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&igResult); err != nil {
		return nil, err
	}

	if igResult.InstagramBusinessAccount.ID == "" {
		return nil, fmt.Errorf("no Instagram Business Account connected to this page")
	}

	// Get Instagram account details
	igDetailsURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s?fields=username,profile_picture_url&access_token=%s",
		igResult.InstagramBusinessAccount.ID, pageToken)

	resp3, err := p.httpClient.Get(igDetailsURL)
	if err != nil {
		return nil, err
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get Instagram details")
	}

	var detailsResult struct {
		Username          string `json:"username"`
		ProfilePictureURL string `json:"profile_picture_url"`
	}

	if err := json.NewDecoder(resp3.Body).Decode(&detailsResult); err != nil {
		return nil, err
	}

	return &AccountInfo{
		AccountID:   igResult.InstagramBusinessAccount.ID,
		AccountName: detailsResult.Username,
		AvatarURL:   detailsResult.ProfilePictureURL,
	}, nil
}

// FetchReviews fetches mentions and comments from Instagram
// Note: Instagram doesn't have a traditional review system, so we fetch mentions and comments
func (p *InstagramProvider) FetchReviews(accessToken string, since time.Time) ([]*Review, error) {
	// Get account info
	accountInfo, err := p.GetAccountInfo(accessToken)
	if err != nil {
		return nil, err
	}

	// Get page access token
	pageToken, err := p.getPageAccessToken(accessToken)
	if err != nil {
		return nil, err
	}

	var allReviews []*Review

	// Fetch media (posts) with comments
	mediaURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/media?fields=id,caption,timestamp,comments_count,like_count&access_token=%s",
		accountInfo.AccountID, pageToken)

	if !since.IsZero() {
		mediaURL += fmt.Sprintf("&since=%d", since.Unix())
	}

	resp, err := p.httpClient.Get(mediaURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch media: %s - %s", resp.Status, string(body))
	}

	var mediaResult struct {
		Data []struct {
			ID            string `json:"id"`
			Caption       string `json:"caption"`
			Timestamp     string `json:"timestamp"`
			CommentsCount int    `json:"comments_count"`
			LikeCount     int    `json:"like_count"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&mediaResult); err != nil {
		return nil, err
	}

	// Fetch comments for each media
	for _, media := range mediaResult.Data {
		if media.CommentsCount == 0 {
			continue
		}

		commentsURL := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/comments?fields=id,text,username,timestamp&access_token=%s",
			media.ID, pageToken)

		resp2, err := p.httpClient.Get(commentsURL)
		if err != nil {
			continue
		}

		if resp2.StatusCode != http.StatusOK {
			resp2.Body.Close()
			continue
		}

		var commentsResult struct {
			Data []struct {
				ID        string `json:"id"`
				Text      string `json:"text"`
				Username  string `json:"username"`
				Timestamp string `json:"timestamp"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp2.Body).Decode(&commentsResult); err != nil {
			resp2.Body.Close()
			continue
		}
		resp2.Body.Close()

		// Convert comments to reviews
		for _, comment := range commentsResult.Data {
			commentTime, _ := time.Parse(time.RFC3339, comment.Timestamp)

			review := &Review{
				PlatformReviewID: comment.ID,
				AuthorName:       comment.Username,
				ReviewText:       comment.Text,
				ReviewedAt:       commentTime,
				Metadata: map[string]interface{}{
					"media_id":      media.ID,
					"media_caption": media.Caption,
					"like_count":    media.LikeCount,
					"type":          "comment",
				},
			}

			allReviews = append(allReviews, review)
		}
	}

	return allReviews, nil
}

// getPageAccessToken gets the page access token needed for Instagram API calls
func (p *InstagramProvider) getPageAccessToken(userAccessToken string) (string, error) {
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

	if len(result.Data) == 0 {
		return "", fmt.Errorf("no pages found")
	}

	return result.Data[0].AccessToken, nil
}
