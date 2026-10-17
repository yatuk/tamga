package linkscan

import (
	"reflect"
	"testing"
)

func TestDestinations(t *testing.T) {
	for text, want := range map[string][]string{
		"send to https://Files.Example.NET:8443/up?x=1 now":                {"url:files.example.net"},
		"sftp://user@backup.example.org/srv and s3://bucket.example.com/k": {"url:backup.example.org", "url:bucket.example.com"},
		"to: ayse.kaya@Mail.Example.com, cc mehmet@example.org":            {"email:mail.example.com", "email:example.org"},
		"https://allowed.example/?to=x@leak.example":                       {"url:allowed.example", "email:leak.example"},
		"http://[2001:db8::1]:80/x":                                        {"url:[2001:db8::1]"},
		"https://%63ollect.example/":                                       {"url:collect.example"},
		"file:///etc/passwd, a relative /path, user@localhost, no@tld.c":   nil,
		"plain text": nil,
	} {
		var got []string
		for _, d := range Destinations(text) {
			got = append(got, d.Kind+":"+d.Host)
			if d.Start < 0 || d.End > len(text) || d.Start >= d.End {
				t.Fatalf("%q: span %d..%d", text, d.Start, d.End)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Destinations(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestHostValue(t *testing.T) {
	for value, want := range map[string]string{
		"files.example.net":     "files.example.net",
		" DB.internal.example ": "db.internal.example",
		"10.0.4.9":              "10.0.4.9",
		"10.0.4.9:22":           "10.0.4.9",
		"[2001:db8::1]":         "[2001:db8::1]",
		"example.com:443":       "example.com",
		"report.pdf":            "report.pdf",
		"v1.2":                  "",
		"localhost":             "",
		"two words.example":     "",
		"a/b.example":           "",
		"":                      "",
	} {
		got, ok := HostValue(value)
		if got != want || ok != (want != "") {
			t.Errorf("HostValue(%q) = %q, %v; want %q", value, got, ok, want)
		}
	}
}

func TestHostList(t *testing.T) {
	l, err := NewHostList([]string{"example.com", "*.internal.example", "10.0.4.9"})
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]bool{
		"example.com": true, "mail.example.com": true, "EXAMPLE.COM.": true, "example.com.evil.example": false,
		"internal.example": false, "db.internal.example": true, "10.0.4.9": true, "10.0.4.10": false,
	} {
		if got := l.Has(host); got != want {
			t.Errorf("Has(%q) = %v", host, got)
		}
	}
	if _, err := NewHostList([]string{"https://example.com"}); err == nil {
		t.Fatal("an entry with a scheme was accepted")
	}
	var none *HostList
	if none.Has("example.com") {
		t.Fatal("a nil list allows a host")
	}
}
