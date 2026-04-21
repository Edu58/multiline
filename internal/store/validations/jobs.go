package validations

import (
	"errors"
	"time"

	"github.com/Edu58/multiline/internal/store/sqlc"
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
		validation.Field(&arg.ShardID, validation.Required),
		validation.Field(&arg.NextRunTime, validation.Required,
			validation.Min(time.Now().UTC()),
			validation.By(validatePgTimestamp),
		),
	)
}

func validatePgTimestamp(value any) error {
	ts, ok := value.(pgtype.Timestamptz)
	if !ok {
		return nil
	}

	if ts.Time.Location() != time.UTC {
		return errors.New("must be in UTC timezone")
	}

	return nil
}
