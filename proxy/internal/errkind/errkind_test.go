package errkind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type quoting struct{ text string }

func (e *quoting) Error() string { return "bad input: " + e.text }

func TestOf(t *testing.T) {
	const text = "TC 38461027540"
	var syntax *json.SyntaxError
	_ = json.Unmarshal([]byte("{"), &struct{}{})
	jsonErr := json.Unmarshal([]byte(`{"a":`+text+`}`), &struct{}{})
	if !errors.As(jsonErr, &syntax) {
		t.Fatalf("setup: %v", jsonErr)
	}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"deadline", fmt.Errorf("scan of %q: %w", text, context.DeadlineExceeded), Timeout},
		{"cancelled", fmt.Errorf("scan of %q: %w", text, context.Canceled), Cancelled},
		{"grpc", status.Error(codes.Unavailable, "could not read "+text), "grpc_unavailable"},
		{"grpc two words", status.Error(codes.DeadlineExceeded, text), "grpc_deadline_exceeded"},
		{"wrapped grpc", fmt.Errorf("gRPC Analyze: %w", status.Error(codes.InvalidArgument, text)), "grpc_invalid_argument"},
		{"own type", &quoting{text: text}, "*errkind.quoting"},
		{"wrapped own type", fmt.Errorf("scanner %q: %w", text, &quoting{text: text}), "*errkind.quoting"},
		{"plain", errors.New("failed on " + text), "*errors.errorString"},
		{"joined", errors.Join(errors.New("first"), &quoting{text: text}), "*errkind.quoting"},
		{"decoder", jsonErr, "*json.SyntaxError"},
		{"named", New("circuit_open", "the breaker is open"), "circuit_open"},
		{"named, wrapped", fmt.Errorf("classifier: %w", New("circuit_open", "open")), "circuit_open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Of(tc.err)
			if got != tc.want {
				t.Fatalf("Of = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "38461027540") {
				t.Fatalf("the kind holds the error's text: %q", got)
			}
		})
	}
}

type longKind struct{}

func (longKind) Error() string     { return "x" }
func (longKind) ErrorKind() string { return strings.Repeat("k", 500) }

func TestOf_IsBounded(t *testing.T) {
	if got := Of(longKind{}); len(got) != maxLen {
		t.Fatalf("len = %d, want %d", len(got), maxLen)
	}
}
