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
	err := os.WriteFile(tmpFile, []byte("HOST=localhost\nPORT=4000\nDSN_URL=DATABASE-URL"), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	expectedConfig := Config{
		HOST:           "localhost",
		PORT:           "4000",
		DSN_URL:        "DATABASE-URL",
		DSN_OPTIONS:    "",
		MIGRATIONS_URL: "",
	}

	actualConfig, err := LoadConfig(tmpDir, ".env.test", "env")
	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, actualConfig)
}
