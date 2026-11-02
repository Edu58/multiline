package multiline

import (
	"context"
	"time"

	"github.com/Edu58/multiline/internal/timewheel"
	"github.com/Edu58/multiline/store"
	"github.com/Edu58/multiline/store/sqlc"
	"github.com/Edu58/multiline/workers"
	"github.com/sirupsen/logrus"
)

type JOBS_RANGE int

const (
	SECONDS JOBS_RANGE = iota
	MINUTES
	HOURS
)

type Scheduler struct {
	ID             any
	TimingWheel    *timewheel.TimeWheel
	store          *store.Store
	workerRegistry *workers.WorkerRegistry
	Dispatcher     *workers.Dispatcher
	PollInterval   time.Duration
	pollTracker    map[string]int64
	logger         *logrus.Logger
	ctx            context.Context
}

func NewScheduler(
	ctx context.Context,
	id any,
	pollInterval time.Duration,
	store *store.Store,
	workerRegistry *workers.WorkerRegistry,
	dispatcher *workers.Dispatcher,
	logger *logrus.Logger,
) *Scheduler {

	ticker := time.NewTicker(time.Second)
	now := time.Now().Unix()

	timeWheel := timewheel.NewTimeWheel(ctx, ticker, store, workerRegistry, dispatcher, logger)
	timeWheel.WithSecondsWheel(timewheel.NewWheel(60, time.Second))
	timeWheel.WithMinutesWheel(timewheel.NewWheel(60, time.Minute))
	timeWheel.WithHoursWheel(timewheel.NewWheel(24, time.Hour))

	pollTracker := map[string]int64{
		"seconds": now,
		"minutes": now,
		"hours":   now,
	}

	return &Scheduler{
		ID:             id,
		TimingWheel:    timeWheel,
		store:          store,
		PollInterval:   pollInterval,
		pollTracker:    pollTracker,
		workerRegistry: workerRegistry,
		Dispatcher:     dispatcher,
		logger:         logger,
		ctx:            ctx,
	}
}

func (s *Scheduler) Start() {
	go s.TimingWheel.Start(s.ctx)
	go s.Poll()
}

func (s *Scheduler) Poll() {
	ticker := time.NewTicker(s.PollInterval)

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("canceling new jobs poll")
			ticker.Stop()

			return
		case <-ticker.C:

			now := time.Now().Unix()

			if now-s.pollTracker["minutes"] >= 60 {
				s.GetJobs(MINUTES)
				s.pollTracker["minutes"] = now
			}

			if now-s.pollTracker["hours"] >= 3600 {
				s.GetJobs(HOURS)
				s.pollTracker["hours"] = now
			}

			s.GetJobs(SECONDS)
			s.pollTracker["seconds"] = now
		}
	}
}

func (s *Scheduler) GetJobs(r JOBS_RANGE) {
	switch r {
	case MINUTES:
		jobs, err := s.store.Queries.GetJobsByWindow(s.ctx, sqlc.GetJobsByWindowParams{
			Status:    new("pending"),
			StartTime: time.Now().UTC().Add(time.Minute).Add(time.Second),
			EndTime:   time.Now().UTC().Add(time.Hour),
		})

		if err != nil {
			s.logger.WithError(err).Error("error getting next hour(minutes bucket) jobs")
			s.ctx.Done()
		}

		s.AddJobs(jobs)

	case HOURS:
		jobs, err := s.store.Queries.GetJobsByWindow(s.ctx, sqlc.GetJobsByWindowParams{
			Status:    new("pending"),
			StartTime: time.Now().UTC().Add(time.Hour).Add(time.Second),
			EndTime:   time.Now().UTC().Add(time.Hour * 24),
		})

		if err != nil {
			s.logger.WithError(err).Error("error getting next 24 hours(hours bucket) jobs")
			s.ctx.Done()
		}

		s.AddJobs(jobs)
	default:
		jobs, err := s.store.Queries.GetNextMinuteJobs(s.ctx,
			sqlc.GetNextMinuteJobsParams{
				Status:  new("pending"),
				EndTime: time.Now().UTC().Add(time.Minute),
			})

		if err != nil {
			s.logger.WithError(err).Error("error getting next minute(seconds bucket) jobs")
			s.ctx.Done()
		}

		s.AddJobs(jobs)
	}
}

func (s *Scheduler) AddJobs(jobs []sqlc.Jobs) {
	if len(jobs) < 1 {
		s.logger.Info("0 pending jobs found. skipping...")
		return
	}

	s.logger.Infof("Adding %d jobs to timewheel", len(jobs))

	for _, job := range jobs {
		s.TimingWheel.AddJob(&timewheel.Job{
			Id:         job.ID,
			JobType:    job.Type,
			Payload:    job.Payload,
			Expiration: job.NextRunTime.Unix(),
		})
	}
}
