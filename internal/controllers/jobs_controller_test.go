package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/services"
	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/Edu58/multiline/pkg/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestJobsController_Index(t *testing.T) {
	mux := http.NewServeMux()

	appConfig, err := config.LoadConfig("../../", "app", "env")

	if err != nil {
		t.Fatal(err)
	}

	db, err := store.New(context.Background(), &logrus.Logger{}, appConfig.DSN_URL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	logger, err := logger.New(&logrus.TextFormatter{}, logger.LoggerOptions{Out: "", Level: ""})
	assert.NoError(t, err)

	store, err := store.New(context.Background(), logger, appConfig.DSN_URL)
	assert.NoError(t, err)

	jobsService := services.NewJobsService(store, logger)
	jobsController := NewJobsController(logger, jobsService)
	jobsController.RegisterRoutes(mux)

	req, err := http.NewRequest("GET", "/jobs/list", nil)

	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
	})

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, recorder.Code)
	}

	expectedResponse := []struct {
		Limit  int32
		Offset int32
	}{
		{10, 0},
		{5, 5},
	}

	for _, tt := range expectedResponse {
		req = reqWithContextValue(tt.Limit, tt.Offset)
		handler.ServeHTTP(recorder, req)

		expectedJobs := []sqlc.Jobs{
			{
				ID:          uuid.New(),
				Name:        "Job1",
				Description: pgtype.Text{String: "Description1", Valid: true},
				Type:        "type1",
				Schedule:    "schedule1",
				LastRunTime: pgtype.Timestamptz{Time: time.Now()},
				NextRunTime: pgtype.Timestamptz{Time: time.Now()},
				Payload:     nil,
				Status:      pgtype.Text{String: "active"},
				ShardID:     pgtype.Int4{Int32: 1},
			},
			{
				ID:          uuid.New(),
				Name:        "Job2",
				Description: pgtype.Text{String: "Description2", Valid: true},
				Type:        "type2",
				Schedule:    "schedule2",
				LastRunTime: pgtype.Timestamptz{Time: time.Now()},
				NextRunTime: pgtype.Timestamptz{Time: time.Now()},
				Payload:     nil,
				Status:      pgtype.Text{String: "active"},
				ShardID:     pgtype.Int4{Int32: 2},
			},
		}

		var actualJobs []sqlc.Jobs
		err = json.NewDecoder(recorder.Body).Decode(&actualJobs)
		if err != nil {
			t.Errorf("failed to decode JSON response: %v", err)
		}
		if !reflect.DeepEqual(actualJobs, expectedJobs) {
			t.Errorf("expected jobs %v, got %v", expectedJobs, actualJobs)
		}
	}
}

func reqWithContextValue(limit, offset int32) *http.Request {
	req, _ := http.NewRequest("GET", "/jobs/list", nil)
	ctx := context.WithValue(context.Background(), "userEmail", limit)
	ctx = context.WithValue(ctx, "userEmail", offset)

	return req.WithContext(ctx)
}
