package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Edu58/multiline/config"
	apphttp "github.com/Edu58/multiline/internal/app_http"
	"github.com/Edu58/multiline/internal/store"
	"github.com/sirupsen/logrus"
)

type App struct {
	config *config.Config
	store  *store.Store
	server *http.Server
	mux    *http.ServeMux
}

func NewApp(config *config.Config, store *store.Store) (*App, error) {
	mux := http.NewServeMux()
	addr := config.HOST + ":" + config.PORT

	return &App{
		config: config,
		store:  store,
		mux:    mux,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}, nil
}

func (app *App) Init() error {
	logrus.Info("Setting up routes")
	apphttp.NewDefaultHandler().RegisterRoutes(app.mux)
	return nil
}

func (app *App) Start() error {
	logrus.WithField("addr", app.server.Addr).Info("Starting server")
	return app.server.ListenAndServe()
}

func (app *App) Shutdown(ctx context.Context, waitForShutdownCompletion chan struct{}) error {
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigch

	logrus.Printf("Got signal: %v . Server shutting down.", sig)

	if err := app.server.Shutdown(ctx); err != nil {
		logrus.Errorf("Error during shutdown: %v", err)
		return err
	}

	waitForShutdownCompletion <- struct{}{}
	return nil
}
