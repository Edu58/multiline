package app

import (
	"context"
	"testing"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/pkg/logger"
	"github.com/sirupsen/logrus"
)

func TestApp_Start(t *testing.T) {
	appConfig, err := config.LoadConfig("../../", "app", "env")

	if err != nil {
		t.Fatal(err)
	}

	db, err := store.New(context.Background(), &logrus.Logger{}, appConfig.DSN_URL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var loggerOptions = logger.LoggerOptions{
		Out:   "",
		Level: "debug",
	}

	logger, err := logger.New(&logrus.JSONFormatter{}, loggerOptions)

	if err != nil {
		t.Fatalf("failed to create logger for test: %v", err)
	}

	_, err = NewApp(db, &appConfig, logger)
	if err != nil {
		t.Fatal(err)
	}

	// startChan := make(chan error)

	// go func() {
	// 	err := app.Start()
	// 	if err != nil {
	// 		log.Printf("Error starting app: %v", err)
	// 		startChan <- err
	// 	} else {
	// 		startChan <- nil
	// 	}
	// }()

	// select {
	// case startErr := <-startChan:
	// 	if startErr != nil {
	// 		t.Errorf("app.Start failed with error: %v", startErr)
	// 	}
	// case <-time.After(5 * time.Second):
	// 	t.Fatal("timeout waiting for server to start")
	// default:
	// }

	// err = app.Shutdown(context.Background(), make(chan struct{}))
	// if err != nil {
	// 	t.Error(err)
	// }
}
