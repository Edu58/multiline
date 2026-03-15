package store

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/Edu58/multiline/config"
	"github.com/Edu58/multiline/internal/store/sqlc"
	store "github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/Edu58/multiline/pkg/strings"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfig config.Config
var testStore *Store

// runs once for the entire package, not per test
// m.Run() is where all the actual tests execute
// everything before is setup, everything after is teardown
func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testConfig, err = config.LoadConfig("../../", "app", "env")

	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}

	testStore, err = New(ctx, testConfig.DSN_URL)

	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}

	// run tests
	code := m.Run()

	testStore.Close()
	os.Exit(code)
}

func TestStore(t *testing.T) {

	var tests = []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"Valid connection", testConfig.DSN_URL, false},
		{"Invalid connection", "invalid_url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := New(context.Background(), tt.url)

			if tt.wantErr {
				assert.Error(t, err, "expected error but got nil")
				return
			}

			require.NoError(t, err, "unexpected error: %v", err)
			defer db.Close()
		})
	}

	t.Run("Transaction succeeds", func(t *testing.T) {

		err := testStore.WithTx(context.Background(), func(q *sqlc.Queries) error {

			jobId := uuid.New()

			_, err := q.CreateOrUpdateJob(context.Background(), store.CreateOrUpdateJobParams{
				ID:       jobId,
				Name:     "Test Job 1",
				Schedule: "0 * * * *",
			})

			if err != nil {
				return err
			}

			_, err = q.GetJob(context.Background(), jobId)
			return err
		})

		assert.NoError(t, err, "transaction failed: %v", err)
	})

	t.Run("Transaction fails", func(t *testing.T) {
		err := testStore.WithTx(context.Background(), func(q *sqlc.Queries) error {

			_, err := q.CreateOrUpdateJob(context.Background(), store.CreateOrUpdateJobParams{
				ID:       uuid.Nil,
				Type:     strings.RandomString(100),
				Schedule: "0 * * * *",
			})

			return err
		})

		assert.Error(t, err, "expected transaction to fail but it succeeded")
	})
}
