package scheduler

// import (
// 	"context"
// 	"encoding/json"
// 	"testing"
// 	"time"

// 	"github.com/Edu58/multiline/config"
// 	"github.com/Edu58/multiline/internal/store"
// 	"github.com/Edu58/multiline/pkg/logger"
// 	"github.com/sirupsen/logrus"
// 	"github.com/stretchr/testify/assert"
// )

// func TestNewJob(t *testing.T) {
// 	jobType := "email"
// 	expiration := time.Second * 45
// 	payload, err := json.Marshal(map[string]any{
// 		"name":    "Test User",
// 		"email":   "testuser@gmail.com",
// 		"message": "We got billions now",
// 	})

// 	assert.NoError(t, err)

// 	job := NewJob(jobType, payload, expiration)

// 	assert.NotNil(t, job)
// 	assert.WithinRange(t, time.Unix(0, job.expiration).UTC(), time.Now().UTC().Add(time.Second*44), time.Now().UTC().Add(time.Second*46))
// }

// func TestAddJob(t *testing.T) {
// 	appConfig, err := config.LoadConfig("../../", "app", "env")

// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	db, err := store.New(context.Background(), &logrus.Logger{}, appConfig.DSN_URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer db.Close()

// 	logger, err := logger.New(&logrus.TextFormatter{}, logger.LoggerOptions{Out: "", Level: ""})
// 	assert.NoError(t, err)

// 	store, err := store.New(context.Background(), logger, appConfig.DSN_URL)
// 	assert.NoError(t, err)

// 	payload, err := json.Marshal(map[string]any{
// 		"name":    "Test User",
// 		"email":   "testuser@gmail.com",
// 		"message": "We got billions now",
// 	})

// 	assert.NoError(t, err)

// 	shortJob := NewJob("email", payload, time.Second*2)

// 	assert.NotNil(t, shortJob)

// 	ticker := time.NewTicker(time.Second)
// 	defer ticker.Stop()

// 	secondsWheel := NewWheel(60, time.Second)

// 	scheduler := NewTimeWheel(t.Context(), ticker, store, logger)
// 	scheduler.WithSecondsWheel(secondsWheel)
// 	scheduler.WithMinutesWheel(NewWheel(60, time.Minute))
// 	scheduler.WithHoursWheel(NewWheel(24, time.Hour))
// 	scheduler.Start(t.Context())

// 	err = scheduler.AddJob(shortJob)

// 	assert.NoError(t, err)

// 	position := calculateBucketIdx(
// 		secondsWheel.position,
// 		secondsWheel.interval,
// 		secondsWheel.size,
// 		shortJob.expiration)

// 	assert.Greater(t, position, int64(0))

// 	bucket := secondsWheel.buckets[position]
// 	assert.Equal(t, 1, bucket.jobs.Len())
// }
