package app

import (
	"context"
	"net/http"
	"time"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/controllers"
	"github.com/Edu58/multiline/internal/scheduler"
	"github.com/Edu58/multiline/internal/services"
	"github.com/Edu58/multiline/internal/store"
	"github.com/sirupsen/logrus"
)

type App struct {
	config      *config.Config
	store       *store.Store
	server      *http.Server
	mux         *http.ServeMux
	logger      *logrus.Logger
	jobsService *services.JobsService
	scheduler   *scheduler.Scheduler
}

func NewApp(store *store.Store, config *config.Config, logger *logrus.Logger) (*App, error) {
	addr := config.HOST + ":" + config.PORT
	mux := http.NewServeMux()

	return &App{
		config: config,
		store:  store,
		mux:    mux,
		logger: logger,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}, nil
}

func (app *App) InitScheduler(ctx context.Context) {
	app.logger.Info("Setting up scheduler")
	scheduler := scheduler.NewScheduler(ctx, "Test", 1, time.Second*3, app.store, app.logger)
	scheduler.Start()
	app.scheduler = scheduler
}

func (app *App) InitServices() {
	app.logger.Info("Setting up services")
	app.jobsService = services.NewJobsService(app.store, app.scheduler, app.logger)
}

func (app *App) InitHandlers() {
	app.logger.Info("Setting up routes")
	controllers.NewJobsController(app.logger, app.jobsService).RegisterRoutes(app.mux)
}

func (app *App) Start() error {
	app.logger.WithField("addr", app.server.Addr).Info("Starting server")
	return app.server.ListenAndServe()
}

func (app *App) Shutdown(ctx context.Context) error {
	app.logger.Print("Server shutting down.")

	if err := app.server.Shutdown(ctx); err != nil {
		app.logger.Errorf("Error during shutdown: %v", err)
		return err
	}

	return nil
}
