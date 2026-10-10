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
	// Iss and Aud say who signed the token and what it is for. CreateToken
	// fills them and ParseToken refuses a token that carries anything else,
	// one signed before they existed included.
	Iss string `json:"iss"`
	Aud string `json:"aud"`
	// Nbf is never set by CreateToken; a token that carries it is refused
	// until then.
	Nbf int64 `json:"nbf,omitempty"`
}

const (
	// Issuer and Audience are the only values ParseToken accepts. The
	// secret is the proxy's own and nothing else signs with it, so they do
	// not tell deployments apart; they keep a token signed with the same
	// secret for some other purpose from being a session.
	Issuer   = "tamga"
	Audience = "tamga-management"

	// Leeway is how far the clocks of two replicas may differ: a token
	// issued by one is read by another.
	Leeway = 60 * time.Second

	// tokenAlg is the algorithm CreateToken signs with and the only one
	// ParseToken accepts.
	tokenAlg = "HS256"
)

// CreateToken signs a JWT with the given secret and claims.
// Returns the compact JWT string (header.payload.signature).
func CreateToken(claims Claims, secret []byte) (string, error) {
	// The issue time is the caller's when it gives one (see the session
	// revocation in the api package, which must date a token after the
	// moment it revoked the ones before).
	if claims.Iat == 0 {
		claims.Iat = time.Now().Unix()
	}
	claims.Iss = Issuer
	claims.Aud = Audience
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

	// The signature is HMAC-SHA256 whatever the header says, so a header
	// naming "none" or another algorithm cannot change how the token is
	// verified. It is refused all the same: nothing this package signed
	// carries one.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidFormat
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrInvalidFormat
	}
	if header.Alg != tokenAlg || (header.Typ != "" && header.Typ != "JWT") {
		return nil, ErrInvalidToken
	}

	now := time.Now()
	if now.Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}
	if claims.Iss != Issuer || claims.Aud != Audience {
		return nil, ErrInvalidToken
	}
	// A token without an issue time cannot be compared with a revocation,
	// and one from the future is not one this proxy issued.
	limit := now.Add(Leeway).Unix()
	if claims.Iat <= 0 || claims.Iat > limit || claims.Nbf > limit {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

func signHS256(data, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return mac.Sum(nil)
}
