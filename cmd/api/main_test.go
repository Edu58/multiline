package main

import (
	"context"
	"testing"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/app"
	"github.com/Edu58/multiline/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestAppInitialization(t *testing.T) {
	testConfig, err := config.LoadConfig("../../", "app", "env")
	assert.NoError(t, err, "Failed to load test configuration")

	db, err := store.New(context.Background(), testConfig.DSN_URL)
	assert.NoError(t, err, "Failed to connect to database")

	defer db.Close()

	app, err := app.NewApp(&testConfig, db)
	assert.NoError(t, err, "Failed to create app")

	if err := app.Init(); err != nil {
		t.Errorf("Error initializing app: %v", err)
	}
}
