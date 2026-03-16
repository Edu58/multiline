package strings

import (
	"crypto/rand"
	"errors"
)

var STRING_GENERATOR_LENGTH_ERROR = errors.New("n should be > 1")
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func RandomString(n int) (string, error) {
	if n < 1 {
		return "", STRING_GENERATOR_LENGTH_ERROR
	}
	bytes := make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}
	return string(bytes), nil
}
