package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Edu58/multiline/internal/services"
	"github.com/Edu58/multiline/internal/store/sqlc"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/sirupsen/logrus"
)

type JobsController struct {
	logger      *logrus.Logger
	jobsService *services.JobsService
}

func NewJobsController(logger *logrus.Logger, jobsService *services.JobsService) *JobsController {
	return &JobsController{logger, jobsService}
}

func (c *JobsController) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/jobs", http.HandlerFunc(c.index))
}

func (c *JobsController) index(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		limitParam := r.URL.Query().Get("limit")
		offsetParam := r.URL.Query().Get("offset")

		limit, err := strconv.ParseInt(limitParam, 10, 32)
		offset, err := strconv.ParseInt(offsetParam, 10, 32)

		jobs, err := c.jobsService.ListJobs(
			r.Context(),
			sqlc.ListJobsParams{Limit: int32(limit), Offset: int32(offset)},
		)

		if err != nil {
			validationErrs, ok := err.(validation.Errors)
			if ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(validationErrs)
				return
			}

			c.logger.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")

		err = json.NewEncoder(w).Encode(jobs)

		if err != nil {
			http.Error(w, "error processing request", http.StatusInternalServerError)
			return
		}
		
		return
	}
}
