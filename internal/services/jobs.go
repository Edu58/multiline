package services

import (
	"context"
	"encoding/json"

	"github.com/Edu58/multiline/internal/scheduler"
	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/Edu58/multiline/internal/store/validations"
	"github.com/sirupsen/logrus"
)

type JobsService struct {
	store     *store.Store
	scheduler *scheduler.Scheduler
	logger    *logrus.Logger
}

func NewJobsService(store *store.Store, scheduler *scheduler.Scheduler, logger *logrus.Logger) *JobsService {
	return &JobsService{store, scheduler, logger}
}

func (j *JobsService) ListJobs(ctx context.Context, arg sqlc.ListJobsParams) ([]sqlc.Jobs, error) {
	if err := validations.ListJobs(arg); err != nil {
		return []sqlc.Jobs{}, err
	}
	return j.store.Queries.ListJobs(ctx, arg)
}

func (j *JobsService) CreateJob(ctx context.Context, arg sqlc.CreateOrUpdateJobParams) (sqlc.Jobs, error) {
	if err := validations.CreateJob(arg); err != nil {
		return sqlc.Jobs{}, err
	}

	j.logger.Printf("creating job: %v", arg.Name)

	job, err := j.store.Queries.CreateOrUpdateJob(ctx, arg)

	if err != nil {
		return sqlc.Jobs{}, err
	}

	j.logger.Printf("created job: %v", job.Name)

	var payload map[string]any
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		j.logger.WithError(err).Error("error unmarshaling payload")

		return sqlc.Jobs{}, err
	}

	return job, err
}
