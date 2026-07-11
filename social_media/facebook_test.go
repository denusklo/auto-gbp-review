package socialmedia

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubTransport intercepts all requests, records them, and returns a canned response.
type stubTransport struct {
	gotURL  string
	gotForm map[string]string
	respond string
	status  int
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.gotURL = req.URL.Scheme + "://" + req.URL.Host + req.URL.Path
	body, _ := io.ReadAll(req.Body)
	s.gotForm = map[string]string{}
	for _, pair := range strings.Split(string(body), "&") {
		if k, v, ok := strings.Cut(pair, "="); ok {
			s.gotForm[k] = v
		}
	}
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.respond)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func newStubbedProvider(respond string, status int) (*FacebookProvider, *stubTransport) {
	st := &stubTransport{respond: respond, status: status}
	p := NewFacebookProvider("app-id", "app-secret", "http://localhost/cb")
	p.httpClient = &http.Client{Transport: st}
	return p, st
}

func TestPublishPostTextGoesToFeed(t *testing.T) {
	p, st := newStubbedProvider(`{"id":"123_456"}`, http.StatusOK)
	postID, err := p.PublishPost("123", "tok", "hello world", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postID != "123_456" {
		t.Errorf("postID = %q, want 123_456", postID)
	}
	if !strings.HasSuffix(st.gotURL, "/123/feed") {
		t.Errorf("URL = %q, want .../123/feed", st.gotURL)
	}
	if st.gotForm["message"] != "hello+world" {
		t.Errorf("message = %q", st.gotForm["message"])
	}
}

func TestPublishPostPhotoGoesToPhotosAndPrefersPostID(t *testing.T) {
	p, st := newStubbedProvider(`{"id":"photo9","post_id":"123_789"}`, http.StatusOK)
	postID, err := p.PublishPost("123", "tok", "caption", "http://example.com/pic.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postID != "123_789" {
		t.Errorf("postID = %q, want 123_789 (post_id preferred over id)", postID)
	}
	if !strings.HasSuffix(st.gotURL, "/123/photos") {
		t.Errorf("URL = %q, want .../123/photos", st.gotURL)
	}
	if st.gotForm["caption"] != "caption" {
		t.Errorf("caption = %q", st.gotForm["caption"])
	}
}

func TestPublishPostAPIErrorSurfaced(t *testing.T) {
	p, _ := newStubbedProvider(`{"error":{"message":"(#200) permission denied"}}`, http.StatusForbidden)
	_, err := p.PublishPost("123", "tok", "hello", "")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("want error containing Graph API message, got %v", err)
	}
}
