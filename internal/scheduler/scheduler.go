package scheduler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/sirupsen/logrus"
)

type JOBS_RANGE int

const (
	SECONDS JOBS_RANGE = iota
	MINUTES
	HOURS
)

type Scheduler struct {
	ID           any
	ShardID      any
	TimingWheel  *TimeWheel
	store        *store.Store
	PollInterval time.Duration
	pollTracker  map[string]int64
	logger       *logrus.Logger
}

func NewScheduler(id any, shardID any, pollInterval time.Duration, store *store.Store, logger *logrus.Logger) *Scheduler {
	ticker := time.NewTicker(time.Second)
	now := time.Now().Unix()

	timeWheel := NewTimeWheelScheduler(ticker)
	timeWheel.WithSecondsWheel(NewWheel(60, time.Second))
	timeWheel.WithMinutesWheel(NewWheel(60, time.Minute))
	timeWheel.WithHoursWheel(NewWheel(24, time.Hour))

	pollTracker := map[string]int64{
		"seconds": now,
		"minutes": now,
		"hours":   now,
	}

	return &Scheduler{
		ID:           id,
		ShardID:      shardID,
		TimingWheel:  timeWheel,
		store:        store,
		PollInterval: pollInterval,
		pollTracker:  pollTracker,
		logger:       logger,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.TimingWheel.Start(ctx)
	go s.Poll(ctx)
}

func (s *Scheduler) Poll(ctx context.Context) {
	ticker := time.NewTicker(s.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Canceling poll")
			return
		case <-ticker.C:

			now := time.Now().Unix()

			if now-s.pollTracker["minutes"] >= 60 {
				s.GetJobs(ctx, MINUTES)
				s.pollTracker["minutes"] = now
			}

			if now-s.pollTracker["hours"] >= 3600 {
				s.GetJobs(ctx, HOURS)
				s.pollTracker["hours"] = now
			}

			s.GetJobs(ctx, SECONDS)
			s.pollTracker["seconds"] = now
		}
	}
}

func (s *Scheduler) GetJobs(ctx context.Context, r JOBS_RANGE) {

	switch r {
	case MINUTES:
		jobs, err := s.store.Queries.GetNextHourJobs(ctx)

		if err != nil {
			s.logger.WithError(err).Error("error getting next hour(minutes bucket) jobs")
			ctx.Done()
		}

		s.AddJobs(jobs)

	case HOURS:
		jobs, err := s.store.Queries.GetNext24HourJobs(ctx)

		if err != nil {
			s.logger.WithError(err).Error("error getting next 24 hours(hours bucket) jobs")
			ctx.Done()
		}

		s.AddJobs(jobs)
	default:
		jobs, err := s.store.Queries.GetNextMinuteJobs(ctx)

		if err != nil {
			s.logger.WithError(err).Error("error getting next minute(seconds bucket) jobs")
			ctx.Done()
		}

		s.AddJobs(jobs)
	}
}

func (s *Scheduler) AddJobs(jobs []sqlc.Jobs) {
	if len(jobs) < 1 {
		return
	}

	s.logger.Info("Adding %d jobs to timewheel", len(jobs))

	for _, job := range jobs {
		var payload map[string]any

		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			s.logger.WithError(err).Error("error marshalling job payload")
			continue
		}

		s.TimingWheel.AddJob(&Job{
			id:         job.ID,
			jobType:    job.Type,
			payload:    payload,
			expiration: job.NextRunTime.Unix(),
		})
	}
}
