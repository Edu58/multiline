package strings

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomString(t *testing.T) {
	tests := []struct {
		testLength int
		expected   int
		err        error
	}{
		{5, 5, nil},
		{10, 10, nil},
		{0, 0, STRING_GENERATOR_LENGTH_ERROR},
		{-1, 0, STRING_GENERATOR_LENGTH_ERROR},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.testLength), func(t *testing.T) {
			randomString, err := RandomString(tt.testLength)
			if tt.err != nil {
				assert.EqualError(t, err, STRING_GENERATOR_LENGTH_ERROR.Error())
			}

			assert.Equal(t, tt.expected, len(randomString), "RandomString(%d) expected length %d, got %d", tt.testLength, tt.expected, len(randomString))
		})
	}
}

