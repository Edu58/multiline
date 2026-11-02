package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tmpFile := filepath.Join(tmpDir, ".env.test")
	err := os.WriteFile(tmpFile, []byte("DSN_URL=DATABASE-URL"), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	expectedConfig := Config{
		DSN_URL:        "DATABASE-URL",
		DSN_OPTIONS:    "",
		MIGRATIONS_URL: "",
	}

	actualConfig, err := LoadConfig(tmpDir, ".env.test", "env")
	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, actualConfig)
}
