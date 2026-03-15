package app

import (
	"context"
	"testing"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/store"
)

func TestApp_Start(t *testing.T) {
	appConfig, err := config.LoadConfig("../../", "app", "env")

	if err != nil {
		t.Fatal(err)
	}

	db, err := store.New(context.Background(), appConfig.DSN_URL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	app, err := NewApp(&appConfig, db)
	if err != nil {
		t.Fatal(err)
	}

	err = app.Init()
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

// func TestApp_Shutdown(t *testing.T) {
// 	appConfig, err := config.LoadConfig("", "", "")
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	config, err := pgxpool.ParseConfig(appConfig.DSN_URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	db, err := pgxpool.NewWithConfig(context.Background(), config)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer db.Close()

// 	app, err := NewApp(&appConfig, db)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	err = app.Init()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	startChan := make(chan error)

// 	go func() {
// 		err := app.Start()
// 		if err != nil {
// 			log.Printf("Error starting app: %v", err)
// 			startChan <- err
// 		} else {
// 			startChan <- nil
// 		}
// 	}()

// 	select {
// 	case startErr := <-startChan:
// 		if startErr != nil {
// 			t.Errorf("app.Start failed with error: %v", startErr)
// 		}
// 	case <-time.After(5 * time.Second):
// 		t.Fatal("timeout waiting for server to start")
// 	default:
// 	}

// 	err = app.Shutdown(context.Background(), make(chan struct{}))

// 	if err != nil {
// 		t.Error(err)
// 	}

// 	time.Sleep(100 * time.Millisecond) // Give some time for the server to shut down
// }
