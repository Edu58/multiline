package main

import (
	"context"
	"os"
	"time"

	"github.com/Edu58/multiline/config"

	"github.com/Edu58/multiline/internal/app"
	"github.com/Edu58/multiline/internal/store"
	"github.com/sirupsen/logrus"
)

func init() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
}

func main() {
	appConfig, err := config.LoadConfig(".", "app", "env")

	if err != nil {
		logrus.Fatalf("Could not load config with err: %v", err)
		return
	}

	store, err := store.New(context.Background(), appConfig.DSN_URL)
	if err != nil {
		logrus.Fatalf("Error creating store: %v", err)
	}

	defer store.Close()

	app, err := app.NewApp(&appConfig, store)

	if err != nil {
		logrus.Fatalf("Could create app with err: %v", err)
	}

	if err := app.Init(); err != nil {
		logrus.Fatalf("Error initializing app: %v", err)
	}
	waitForShutdownCompletion := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	go app.Shutdown(ctx, waitForShutdownCompletion)
	defer cancel()

	if err := app.Start(); err != nil {
		logrus.Fatal(err)
	}

	<-waitForShutdownCompletion
}
