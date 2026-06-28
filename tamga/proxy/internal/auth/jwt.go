// Package auth provides JWT token creation/validation and OAuth client utilities
// for the Tamga proxy. It uses HMAC-SHA256 with stdlib crypto — no external
// JWT library dependency — which keeps the binary small and the implementation
// auditable for security review.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrTokenExpired  = errors.New("token expired")
	ErrInvalidFormat = errors.New("invalid token format")
)

// Claims is the JWT payload carried in every Tamga-issued token.
type Claims struct {
	Sub    string `json:"sub"`    // user ID (GitHub user ID)
	Email  string `json:"email"`  // primary email from GitHub
	Name   string `json:"name"`   // display name from GitHub
	Avatar string `json:"avatar"` // avatar URL from GitHub
	Role   string `json:"role"`   // local RBAC role (admin/analyst/viewer)
	Iat    int64  `json:"iat"`    // issued at (unix seconds)
	Exp    int64  `json:"exp"`    // expiration (unix seconds)
}

// CreateToken signs a JWT with the given secret and claims.
// Returns the compact JWT string (header.payload.signature).
func CreateToken(claims Claims, secret []byte) (string, error) {
	claims.Iat = time.Now().Unix()
	if claims.Exp == 0 {
		claims.Exp = time.Now().Add(24 * time.Hour).Unix()
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signingInput := header + "." + payload
	sig := signHS256([]byte(signingInput), secret)
	signature := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + signature, nil
}

// ParseToken validates and returns the claims from a JWT string.
// Returns ErrInvalidToken, ErrTokenExpired, or ErrInvalidFormat on failure.
func ParseToken(token string, secret []byte) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidFormat
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidFormat
	}

	expected := signHS256([]byte(signingInput), secret)
	if !hmac.Equal(signature, expected) {
		return nil, ErrInvalidToken
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidFormat
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrInvalidFormat
	}

	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func signHS256(data, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return mac.Sum(nil)
}
