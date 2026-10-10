package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// forge signs a header and a payload the way CreateToken does, with
// whatever they say.
func forge(t *testing.T, header string, payload map[string]interface{}, secret []byte) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString(raw)
	return input + "." + base64.RawURLEncoding.EncodeToString(signHS256([]byte(input), secret))
}

func goodPayload() map[string]interface{} {
	now := time.Now().Unix()
	return map[string]interface{}{
		"sub": "u1", "role": "admin", "iat": now, "exp": now + 3600, "iss": Issuer, "aud": Audience,
	}
}

const hs256Header = `{"alg":"HS256","typ":"JWT"}`

func TestParseToken_AcceptsWhatCreateTokenSigns(t *testing.T) {
	secret := []byte("test-secret")
	if _, err := ParseToken(forge(t, hs256Header, goodPayload(), secret), secret); err != nil {
		t.Fatalf("a token shaped like CreateToken's was refused: %v", err)
	}
	tok, err := CreateToken(Claims{Sub: "u1", Role: "viewer"}, secret)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(tok, secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Iss != Issuer || claims.Aud != Audience || claims.Iat == 0 {
		t.Errorf("claims = %+v, want issuer, audience and issue time set", claims)
	}
}

// The signature is always checked as HMAC-SHA256, so a header cannot switch
// verification off. A header that names anything else is refused anyway,
// correctly signed or not.
func TestParseToken_RefusesOtherAlgorithms(t *testing.T) {
	secret := []byte("test-secret")
	for _, header := range []string{
		`{"alg":"none","typ":"JWT"}`,
		`{"alg":"None"}`,
		`{"alg":"HS384","typ":"JWT"}`,
		`{"alg":"HS512","typ":"JWT"}`,
		`{"alg":"RS256","typ":"JWT"}`,
		`{"alg":"hs256","typ":"JWT"}`,
		`{"typ":"JWT"}`,
		`{"alg":"HS256","typ":"at+jwt"}`,
		`not json`,
	} {
		// Signed with the real secret: only the header is wrong.
		if _, err := ParseToken(forge(t, header, goodPayload(), secret), secret); err == nil {
			t.Errorf("header %s: accepted", header)
		}
	}

	// The classic "none" token: no signature at all.
	raw, _ := json.Marshal(goodPayload())
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." +
		base64.RawURLEncoding.EncodeToString(raw) + "."
	if _, err := ParseToken(unsigned, secret); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("unsigned alg=none token: %v, want ErrInvalidToken", err)
	}
}

func TestParseToken_RefusesWrongIssuerOrAudience(t *testing.T) {
	secret := []byte("test-secret")
	for name, change := range map[string]func(map[string]interface{}){
		"no issuer":       func(p map[string]interface{}) { delete(p, "iss") },
		"no audience":     func(p map[string]interface{}) { delete(p, "aud") },
		"other issuer":    func(p map[string]interface{}) { p["iss"] = "someone-else" },
		"other audience":  func(p map[string]interface{}) { p["aud"] = "tamga-proxy" },
		"audience a list": func(p map[string]interface{}) { p["aud"] = []string{Audience} },
		// What a token signed before this check carried.
		"neither": func(p map[string]interface{}) { delete(p, "iss"); delete(p, "aud") },
	} {
		p := goodPayload()
		change(p)
		if _, err := ParseToken(forge(t, hs256Header, p, secret), secret); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseToken_Times(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Now().Unix()
	skew := int64(Leeway / time.Second)
	for name, tc := range map[string]struct {
		change func(map[string]interface{})
		ok     bool
	}{
		"no issue time":                   {func(p map[string]interface{}) { delete(p, "iat") }, false},
		"issued within the leeway":        {func(p map[string]interface{}) { p["iat"] = now + skew - 5 }, true},
		"issued past the leeway":          {func(p map[string]interface{}) { p["iat"] = now + skew + 30 }, false},
		"not yet valid":                   {func(p map[string]interface{}) { p["nbf"] = now + skew + 30 }, false},
		"not-before in the past":          {func(p map[string]interface{}) { p["nbf"] = now - 10 }, true},
		"not-before within the leeway":    {func(p map[string]interface{}) { p["nbf"] = now + 5 }, true},
		"expired":                         {func(p map[string]interface{}) { p["exp"] = now - 1 }, false},
		"no expiry":                       {func(p map[string]interface{}) { delete(p, "exp") }, false},
		"issue time that is not a number": {func(p map[string]interface{}) { p["iat"] = "now" }, false},
	} {
		p := goodPayload()
		tc.change(p)
		_, err := ParseToken(forge(t, hs256Header, p, secret), secret)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want accepted = %v", name, err, tc.ok)
		}
	}
}

// The issue time is the caller's when it gives one: the api package dates a
// token after the revocation it follows.
func TestCreateToken_KeepsAGivenIssueTime(t *testing.T) {
	secret := []byte("test-secret")
	want := time.Now().Unix() + 1
	tok, err := CreateToken(Claims{Sub: "u1", Iat: want}, secret)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(tok, secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Iat != want {
		t.Errorf("Iat = %d, want %d", claims.Iat, want)
	}
}
