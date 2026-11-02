package multiline

import (
	"strings"
	"unicode/utf8"
)

// ErrorMessage converts an execution error to bounded, valid UTF-8 text for
// persistence. NUL and invalid UTF-8 become U+FFFD. Messages exceeding
// MaxErrorBytes are truncated at a rune boundary and end with "... [truncated]".
// Nil yields an empty string. Classification and wrapping of err are unchanged;
// adapters must use the original error for retry policy, not this message.
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	var out strings.Builder
	out.Grow(min(len(message), MaxErrorBytes))
	for len(message) > 0 {
		r, size := utf8.DecodeRuneInString(message)
		if r == 0 {
			r = utf8.RuneError
		}
		if out.Len()+utf8.RuneLen(r) > MaxErrorBytes {
			const marker = "... [truncated]"
			text := out.String()
			end := MaxErrorBytes - len(marker)
			for !utf8.RuneStart(text[end]) {
				end--
			}
			return text[:end] + marker
		}
		out.WriteRune(r)
		message = message[size:]
	}
	return out.String()
}
