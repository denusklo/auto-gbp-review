package socialmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// XHSProvider implements the SocialMediaProvider interface for Xiaohongshu
type XHSProvider struct {
	config   *oauth2.Config
	clientID string
}

// XHSAuthURL is the base URL for XHS OAuth authorization
const XHSAuthURL = "https://open.xiaohongshu.com/oauth2/authorize"

// XHSTokenURL is the token endpoint for XHS
const XHSTokenURL = "https://open.xiaohongshu.com/oauth2/access_token"

// XHSAPIBaseURL is the base URL for XHS API
const XHSAPIBaseURL = "https://open.xiaohongshu.com"

// XHSAccountInfo represents XHS user account information
type XHSAccountInfo struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Gender   int    `json:"gender"`
	Desc     string `json:"desc"`
}

// XHSNote represents a Xiaohongshu note (post)
type XHSNote struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Desc        string        `json:"desc"`
	Type        string        `json:"type"`
	LikedCount  int           `json:"liked_count"`
	Collected   int           `json:"collected"`
	CommentCount int          `json:"comment_count"`
	ShareCount  int           `json:"share_count"`
	Time        int64         `json:"time"`
	Images      []XHSImage    `json:"images"`
	Tags        []string      `json:"tags"`
	User        XHSUser       `json:"user"`
}

// XHSComment represents a comment on a XHS note
type XHSComment struct {
	ID          string     `json:"id"`
	Content     string     `json:"content"`
	LikeCount   int        `json:"like_count"`
	Time        int64      `json:"time"`
	User        XHSUser    `json:"user"`
	SubComments []XHSComment `json:"sub_comments"`
}

// XHSUser represents a XHS user
type XHSUser struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// XHSImage represents an image in a XHS note
type XHSImage struct {
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileID   string `json:"file_id"`
}

// XHSAPIResponse represents a standard XHS API response
type XHSAPIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// NewXHSProvider creates a new XHS provider
func NewXHSProvider(clientID, clientSecret, redirectURI string) *XHSProvider {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes:       []string{"snsapi_base", "snsapi_userinfo"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  XHSAuthURL,
			TokenURL: XHSTokenURL,
		},
	}

	return &XHSProvider{
		config:   config,
		clientID: clientID,
	}
}

// GetPlatformName returns the platform name
func (p *XHSProvider) GetPlatformName() string {
	return PlatformXiaohongshu
}

// GetAuthorizationURL returns the OAuth authorization URL
func (p *XHSProvider) GetAuthorizationURL(state string) string {
	// XHS uses a slightly different OAuth flow, so we construct the URL manually
	params := url.Values{}
	params.Add("app_id", p.clientID)
	params.Add("redirect_uri", p.config.RedirectURL)
	params.Add("response_type", "code")
	params.Add("state", state)
	params.Add("scope", strings.Join(p.config.Scopes, " "))

	return fmt.Sprintf("%s?%s", XHSAuthURL, params.Encode())
}

// ExchangeCodeForToken exchanges authorization code for tokens
func (p *XHSProvider) ExchangeCodeForToken(code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("app_id", p.clientID)
	data.Set("app_secret", p.config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", XHSTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &TokenResponse{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// RefreshToken refreshes an access token using refresh token
func (p *XHSProvider) RefreshToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("app_id", p.clientID)
	data.Set("app_secret", p.config.ClientSecret)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", XHSTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &TokenResponse{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// ValidateToken checks if an access token is still valid
func (p *XHSProvider) ValidateToken(accessToken string) (bool, error) {
	// Use a lightweight API call to validate the token
	req, err := http.NewRequest("GET", XHSAPIBaseURL+"/ark/open_api/v1/user/profile", nil)
	if err != nil {
		return false, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetAccountInfo retrieves account information
func (p *XHSProvider) GetAccountInfo(accessToken string) (*AccountInfo, error) {
	req, err := http.NewRequest("GET", XHSAPIBaseURL+"/ark/open_api/v1/user/profile", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create account info request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp XHSAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	// Parse the data field
	dataBytes, _ := json.Marshal(apiResp.Data)
	var xhsInfo XHSAccountInfo
	if err := json.Unmarshal(dataBytes, &xhsInfo); err != nil {
		return nil, fmt.Errorf("failed to parse account info: %w", err)
	}

	return &AccountInfo{
		AccountID:   xhsInfo.UserID,
		AccountName: xhsInfo.Nickname,
		AvatarURL:   xhsInfo.Avatar,
	}, nil
}

// FetchReviews fetches reviews (comments) from XHS
func (p *XHSProvider) FetchReviews(accessToken string, since time.Time) ([]*Review, error) {
	var reviews []*Review

	// First, get the user's notes to find comments
	notes, err := p.getUserNotes(accessToken, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get user notes: %w", err)
	}

	// For each note, fetch comments that could be reviews
	for _, note := range notes {
		noteTime := time.Unix(note.Time, 0)
		if noteTime.Before(since) {
			continue
		}

		comments, err := p.getNoteComments(accessToken, note.ID)
		if err != nil {
			// Log error but continue processing other notes
			continue
		}

		for _, comment := range comments {
			commentTime := time.Unix(comment.Time, 0)
			if commentTime.After(since) {
				review := &Review{
					PlatformReviewID: fmt.Sprintf("note_%s_comment_%s", note.ID, comment.ID),
					AuthorName:       comment.User.Nickname,
					AuthorPhotoURL:   comment.User.Avatar,
					Rating:           nil, // XHS doesn't have star ratings
					ReviewText:       comment.Content,
					ReviewedAt:       commentTime,
					Metadata: map[string]interface{}{
						"note_id":       note.ID,
						"note_title":    note.Title,
						"comment_id":    comment.ID,
						"like_count":    comment.LikeCount,
						"note_type":     note.Type,
						"note_images":   note.Images,
					},
				}
				reviews = append(reviews, review)
			}
		}
	}

	return reviews, nil
}

// getUserNotes fetches user's notes since the specified time
func (p *XHSProvider) getUserNotes(accessToken string, since time.Time) ([]XHSNote, error) {
	// This is a simplified implementation
	// In reality, you'd need to handle pagination properly
	req, err := http.NewRequest("GET", XHSAPIBaseURL+"/ark/open_api/v1/user/notes", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    struct {
			Notes []XHSNote `json:"notes"`
			HasMore bool    `json:"has_more"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	return apiResp.Data.Notes, nil
}

// getNoteComments fetches comments for a specific note
func (p *XHSProvider) getNoteComments(accessToken, noteID string) ([]XHSComment, error) {
	url := fmt.Sprintf("%s/ark/open_api/v1/notes/%s/comments", XHSAPIBaseURL, noteID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    struct {
			Comments []XHSComment `json:"comments"`
			HasMore  bool         `json:"has_more"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	return apiResp.Data.Comments, nil
}

// makeAPIRequest makes a request to the XHS API with proper authentication
func (p *XHSProvider) makeAPIRequest(ctx context.Context, accessToken, method, endpoint string, body interface{}) (*XHSAPIResponse, error) {
	var reqBody io.Reader

	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, XHSAPIBaseURL+endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp XHSAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, err
	}

	return &apiResp, nil
}