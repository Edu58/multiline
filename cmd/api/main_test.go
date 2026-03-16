package main

import (
	"context"
	"testing"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/app"
	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/pkg/logger"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAppInitialization(t *testing.T) {
	testConfig, err := config.LoadConfig("../../", "app", "env")
	assert.NoError(t, err, "Failed to load test configuration")

	logger, _ := logger.New(&logrus.JSONFormatter{}, logger.LoggerOptions{Out: testConfig.LOG_OUT, Level: testConfig.LOG_LEVEL})

	db, err := store.New(context.Background(), logger, testConfig.DSN_URL)
	assert.NoError(t, err, "Failed to connect to database")

	defer db.Close()

	_, err = app.NewApp(db, &testConfig, logger)
	assert.NoError(t, err, "Failed to create app")

}
