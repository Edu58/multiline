package services

import (
	"context"

	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/Edu58/multiline/internal/store/validations"
	"github.com/sirupsen/logrus"
)

type JobsService struct {
	store  *store.Store
	logger *logrus.Logger
}

func NewJobsService(store *store.Store, logger *logrus.Logger) *JobsService {
	return &JobsService{store, logger}
}

func (j *JobsService) ListJobs(ctx context.Context, arg sqlc.ListJobsParams) ([]sqlc.Jobs, error) {
	if err := validations.ListJobs(arg); err != nil {
		return []sqlc.Jobs{}, err
	}
	return j.store.Queries.ListJobs(ctx, arg)
}
