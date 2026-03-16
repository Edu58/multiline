package validations

import (
	"github.com/Edu58/multiline/internal/store/sqlc"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func ListJobs(arg sqlc.ListJobsParams) error {
	return validation.ValidateStruct(&arg,
		validation.Field(&arg.Limit, validation.Required),
	)
}
