package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewJob(t *testing.T) {
	jobType := "email"
	expiration := time.Second * 45
	payload := map[string]any{
		"name":    "Test User",
		"email":   "testuser@gmail.com",
		"message": "We got billions now",
	}

	job := NewJob(jobType, payload, expiration)

	assert.NotNil(t, job)
	assert.WithinRange(t, time.Unix(0, job.expiration).UTC(), time.Now().UTC().Add(time.Second*44), time.Now().UTC().Add(time.Second*46))
}

func TestAddJob(t *testing.T) {
	shortJob := NewJob("email",
		map[string]any{
			"name":    "Test User",
			"email":   "testuser@gmail.com",
			"message": "We got billions now",
		}, time.Second*2)

	assert.NotNil(t, shortJob)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	stop := make(chan struct{})

	secondsWheel := NewWheel(60, time.Second)

	scheduler := NewTimeWheelScheduler(ticker, stop)
	scheduler.WithSecondsWheel(secondsWheel)
	scheduler.WithMinutesWheel(NewWheel(60, time.Minute))
	scheduler.WithHoursWheel(NewWheel(24, time.Hour))
	scheduler.Start()

	err := scheduler.AddJob(shortJob)

	assert.NoError(t, err)

	position := calculateBucketIdx(
		secondsWheel.position,
		secondsWheel.interval,
		secondsWheel.size,
		shortJob.expiration)

	assert.Greater(t, position, int64(0))

	bucket := secondsWheel.buckets[position]
	assert.Equal(t, 1, bucket.jobs.Len())
}
