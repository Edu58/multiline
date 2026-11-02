package multiline_test

import (
	"context"
	"time"

	"github.com/Edu58/multiline"
	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/services"
	"github.com/Edu58/multiline/store"
	"github.com/Edu58/multiline/workers"
	"github.com/sirupsen/logrus"
)

// The host application provides its own implementation of workers.Handler.
func ExampleNewScheduler() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := logrus.New()
	db, err := store.New(ctx, logger, "postgresql://localhost/multiline?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	registry := workers.NewWorkerRegistry()
	// Register application handlers before constructing the dispatcher:
	// registry.Register("email", emailHandler)
	cfg := &config.Config{WORKERS: map[string]string{}}
	// Map each job type to a registered handler, e.g. "EMAIL": "email".
	dispatcher, err := workers.NewDispatcher(cfg, registry)
	if err != nil {
		panic(err)
	}
	s := multiline.NewScheduler(ctx, "scheduler-1", 3*time.Second,
		db, registry, dispatcher, logger)
	jobs := services.NewJobsService(db, s, logger)
	_ = jobs // Use jobs.CreateJob and jobs.ListJobs from your application.
	s.Start()
	// Keep the application running; cancel ctx before closing connections.
}
