package main

import (
	"context"
	"log"
	"time"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/pkg/logger"

	"github.com/Edu58/multiline/internal/app"
	"github.com/Edu58/multiline/internal/store"
	"github.com/sirupsen/logrus"
)

func main() {
	appConfig, err := config.LoadConfig(".", "app", "env")

	if err != nil {
		log.Fatalf("Could not load config with err: %v", err)
	}

	logger, err := logger.New(&logrus.JSONFormatter{}, logger.LoggerOptions{Out: appConfig.LOG_OUT, Level: appConfig.LOG_LEVEL})

	store, err := store.New(context.Background(), logger, appConfig.DSN_URL)
	if err != nil {
		logger.Fatalf("Error creating store: %v", err)
	}

	defer store.Close()

	app, err := app.NewApp(store, &appConfig, logger)

	if err != nil {
		logger.Fatalf("Could create app with err: %v", err)
	}

	app.InitServices()
	app.InitHandlers()

	waitForShutdownCompletion := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	go app.Shutdown(ctx, waitForShutdownCompletion)
	defer cancel()

	if err := app.Start(); err != nil {
		logger.Fatal(err)
	}

	<-waitForShutdownCompletion
}
