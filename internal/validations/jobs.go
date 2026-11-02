package validations

import (
	"errors"
	"time"

	"github.com/Edu58/multiline/store/sqlc"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/jackc/pgx/v5/pgtype"
)

func ListJobs(arg sqlc.ListJobsParams) error {
	return validation.ValidateStruct(&arg,
		validation.Field(&arg.Limit, validation.Required),
	)
}

func CreateJob(arg sqlc.CreateOrUpdateJobParams) error {
	return validation.ValidateStruct(&arg,
		validation.Field(&arg.Type, validation.Required),
		validation.Field(&arg.NextRunTime, validation.Required,
			validation.By(validateNextRuntime),
		),
	)
}

func validateNextRuntime(value any) error {
	ts, ok := value.(pgtype.Timestamptz)
	if !ok {
		return nil
	}

	if ts.Time.Location() != time.UTC {
		return errors.New("must be in UTC timezone")
	}

	if ts.Time.After(time.Now().UTC()) {
		return errors.New("must be in the future")
	}

	return nil
}
