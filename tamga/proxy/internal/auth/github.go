package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubOAuthConfig holds the configuration for GitHub OAuth.
type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
	// HTTPClient is an optional *http.Client for testing or custom transport.
	// When nil, a default client with 10s timeout is used.
	HTTPClient *http.Client
}

// httpClient returns the configured HTTP client, or a sensible default.
func (c GitHubOAuthConfig) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// GitHubUser is the subset of GitHub's user API we need for identity.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// IsConfigured returns true when all required OAuth fields are set.
func (c GitHubOAuthConfig) IsConfigured() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.CallbackURL != ""
}

// AuthorizeURL builds the GitHub OAuth authorize URL (step 1 of the flow).
func (c GitHubOAuthConfig) AuthorizeURL(state string) string {
	u, _ := url.Parse("https://github.com/login/oauth/authorize")
	q := u.Query()
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.CallbackURL)
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}

// ExchangeCode exchanges the OAuth authorization code for a GitHub access token (step 2).
func (c GitHubOAuthConfig) ExchangeCode(ctx context.Context, code string) (string, error) {
	u := "https://github.com/login/oauth/access_token"
	body := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.CallbackURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("token exchange: status %d", resp.StatusCode)
	}

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("token exchange decode: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("github oauth error: %s", out.Error)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	return out.AccessToken, nil
}

// GetUser fetches the authenticated GitHub user (step 3).
// It also fetches the primary email if the user's profile doesn't include a public email.
func (c GitHubOAuthConfig) GetUser(ctx context.Context, accessToken string) (*GitHubUser, error) {
	client := c.httpClient()
	user, err := fetchGitHubUser(ctx, client, accessToken)
	if err != nil {
		return nil, err
	}

	// If no public email, fetch from /user/emails
	if user.Email == "" {
		email, err := fetchGitHubPrimaryEmail(ctx, client, accessToken)
		if err == nil {
			user.Email = email
		}
	}

	// Use user ID as string identifier
	if user.ID == 0 {
		return nil, fmt.Errorf("github user has no id")
	}

	return user, nil
}

func fetchGitHubUser(ctx context.Context, client *http.Client, accessToken string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get user: status %d", resp.StatusCode)
	}

	var user GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("get user decode: %w", err)
	}
	return &user, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, client *http.Client, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	// Fallback: first verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}

	return "", nil
}
