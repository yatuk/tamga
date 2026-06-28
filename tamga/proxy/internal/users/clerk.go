package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ClerkClient fetches user metadata from Clerk's Backend API. It is optional:
// when Secret / Instance are empty the dashboard falls back to displaying
// just the user_id + role from the local store.
type ClerkClient struct {
	Secret string
	HTTP   *http.Client
}

func NewClerkClient(secret string) *ClerkClient {
	if secret == "" {
		return nil
	}
	return &ClerkClient{
		Secret: secret,
		HTTP:   &http.Client{Timeout: 5 * time.Second},
	}
}

// ClerkUser is the subset of the Clerk user object we render on the dashboard.
type ClerkUser struct {
	ID             string `json:"id"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	ImageURL       string `json:"image_url"`
	EmailAddresses []struct {
		EmailAddress string `json:"email_address"`
	} `json:"email_addresses"`
	PrimaryEmail string `json:"-"`
}

func (u *ClerkUser) Name() string {
	n := u.FirstName
	if u.LastName != "" {
		if n != "" {
			n += " "
		}
		n += u.LastName
	}
	return n
}

func (u *ClerkUser) Email() string {
	if u.PrimaryEmail != "" {
		return u.PrimaryEmail
	}
	if len(u.EmailAddresses) > 0 {
		return u.EmailAddresses[0].EmailAddress
	}
	return ""
}

// ListUsers returns all users in the Clerk instance (capped at limit).
func (c *ClerkClient) ListUsers(ctx context.Context, limit int) ([]ClerkUser, error) {
	if c == nil {
		return nil, errors.New("clerk client not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	url := fmt.Sprintf("https://api.clerk.com/v1/users?limit=%d", limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clerk list users: status %d", resp.StatusCode)
	}
	var out []ClerkUser
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
