package postgres_test

import (
	"reflect"
	"testing"

	"github.com/Edu58/multiline"
	"github.com/Edu58/multiline/postgres"
)

func TestTransactionCannotBeSchedulerRepository(t *testing.T) {
	repository := reflect.TypeFor[multiline.Repository]()
	if !reflect.TypeFor[*postgres.Store]().Implements(repository) {
		t.Fatal("pool-backed store must implement Repository")
	}
	if reflect.TypeFor[*postgres.TxStore]().Implements(repository) {
		t.Fatal("transaction-bound store must not implement Repository")
	}
}
