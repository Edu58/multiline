# Integration tests create isolated schemas in a disposable database.
.PHONY: check vet integration

check: build vet test
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))"

vet:
	go vet ./...

integration:
	go test -race -tags=integration ./postgres

.PHONY: build test race migrate fmt sqlc

fmt:
	go fmt ./...

test:
	go test -v -cover ./...

build:
	go build ./...

race:
	go test -race ./...

# The application chooses its database; never embed credentials in this file.
migrate:
	@test -n "$$MULTILINE_DSN" || (echo "MULTILINE_DSN is required"; exit 1)
	migrate -database "$$MULTILINE_DSN" -path ./db/migrations/ -verbose up

sqlc:
	@test "$$(sqlc version)" = "v1.31.1" || (echo "sqlc v1.31.1 is required"; exit 1)
	sqlc generate
