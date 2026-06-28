package auth

import (
	"testing"
	"time"
)

func TestCreateAndParseToken(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!!")
	claims := Claims{
		Sub:   "12345",
		Email: "user@example.com",
		Name:  "Test User",
		Role:  "viewer",
		Exp:   time.Now().Add(1 * time.Hour).Unix(),
	}

	token, err := CreateToken(claims, secret)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	parsed, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if parsed.Sub != "12345" {
		t.Errorf("Sub: want 12345, got %s", parsed.Sub)
	}
	if parsed.Email != "user@example.com" {
		t.Errorf("Email: want user@example.com, got %s", parsed.Email)
	}
	if parsed.Name != "Test User" {
		t.Errorf("Name: want Test User, got %s", parsed.Name)
	}
	if parsed.Role != "viewer" {
		t.Errorf("Role: want viewer, got %s", parsed.Role)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	secret := []byte("secret-a")
	claims := Claims{Sub: "1", Exp: time.Now().Add(1 * time.Hour).Unix()}

	token, err := CreateToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseToken(token, []byte("secret-b"))
	if err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestParseToken_Expired(t *testing.T) {
	secret := []byte("test-secret")
	claims := Claims{
		Sub: "1",
		Exp: time.Now().Add(-1 * time.Hour).Unix(), // expired 1 hour ago
	}

	token, err := CreateToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ParseToken(token, secret)
	if err != ErrTokenExpired {
		t.Fatalf("want ErrTokenExpired, got %v", err)
	}
}

func TestParseToken_InvalidFormat(t *testing.T) {
	secret := []byte("test-secret")

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"one part", "header"},
		{"two parts", "header.payload"},
		{"garbage", "not.a.jwt.token.with.enough.parts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToken(tt.token, secret)
			if err == nil {
				t.Error("expected error for invalid token")
			}
		})
	}
}

func TestParseToken_InvalidBase64(t *testing.T) {
	secret := []byte("test-secret")
	_, err := ParseToken("!!!invalid!!!base64!!!.payload.sig", secret)
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestCreateToken_AutoIat(t *testing.T) {
	secret := []byte("test-secret")
	claims := Claims{Sub: "1", Exp: time.Now().Add(1 * time.Hour).Unix()}

	before := time.Now().Unix()
	token, err := CreateToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Iat < before {
		t.Errorf("Iat %d should be >= before %d", parsed.Iat, before)
	}
}

func TestCreateToken_DefaultExpiry(t *testing.T) {
	secret := []byte("test-secret")
	claims := Claims{Sub: "1"} // Exp = 0

	before := time.Now().Unix()
	token, err := CreateToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseToken(token, secret)
	if err != nil {
		t.Fatal(err)
	}

	// Default expiry should be ~24h from now
	if parsed.Exp-before < 23*3600 {
		t.Errorf("Exp %d too close to now %d", parsed.Exp, before)
	}
}

func TestCreateToken_Deterministic(t *testing.T) {
	// Same claims + same secret + same time = same token (HMAC is deterministic)
	secret := []byte("test-secret")
	claims := Claims{Sub: "1", Iat: 1000000, Exp: 2000000}

	t1, _ := CreateToken(claims, secret)
	t2, _ := CreateToken(claims, secret)

	if t1 != t2 {
		t.Error("same claims should produce same token")
	}
}

func TestParseToken_GarbagePayload(t *testing.T) {
	secret := []byte("test-secret")
	claims := Claims{Sub: "1", Exp: time.Now().Add(1 * time.Hour).Unix()}
	token, _ := CreateToken(claims, secret)

	// Corrupt the middle (payload) part
	parts := []byte(token)
	// Find payload part (between first and second dot)
	for i := range parts {
		if parts[i] == '.' {
			parts[i+1] ^= 0xFF // corrupt first char of payload
			break
		}
	}

	_, err := ParseToken(string(parts), secret)
	if err == nil {
		t.Error("expected error for corrupted token")
	}
}
