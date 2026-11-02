package multiline

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestErrorMessageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		message   string
		truncated bool
	}{
		{"short", false}, {strings.Repeat("x", MaxErrorBytes), false},
		{strings.Repeat("x", MaxErrorBytes+1), true},
		{strings.Repeat("€", MaxErrorBytes/3+1), true},
		{"prefix\x00suffix", false}, {"bad\xffUTF-8", false},
		{strings.Repeat("€", (MaxErrorBytes-20)/3) + strings.Repeat("x", 40), true},
	} {
		actual := ErrorMessage(errors.New(tc.message))
		if len(actual) > MaxErrorBytes || !utf8.ValidString(actual) || strings.ContainsRune(actual, 0) {
			t.Fatalf("invalid persisted text: %q", actual)
		}
		if strings.HasSuffix(actual, "... [truncated]") != tc.truncated {
			t.Fatalf("incorrect truncation for %d-byte input", len(tc.message))
		}
		if !tc.truncated && utf8.ValidString(tc.message) && !strings.ContainsRune(tc.message, 0) && actual != tc.message {
			t.Fatal("changed valid message within limit")
		}
	}
	if ErrorMessage(nil) != "" {
		t.Fatal("nil error must be empty")
	}
	if got := ErrorMessage(errors.New("a\x00b\xffc")); got != "a�b�c" {
		t.Fatalf("sanitized = %q", got)
	}
}

func FuzzErrorMessageBounds(f *testing.F) {
	for _, seed := range []string{"", "short", "a\x00b\xffc", strings.Repeat("€", 2000)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, message string) {
		actual := ErrorMessage(errors.New(message))
		if len(actual) > MaxErrorBytes || !utf8.ValidString(actual) || strings.ContainsRune(actual, 0) {
			t.Fatal("error text violates persistence limits")
		}
		if len(message) <= MaxErrorBytes && utf8.ValidString(message) && !strings.ContainsRune(message, 0) && actual != message {
			t.Fatal("changed valid input within limit")
		}
	})
}
