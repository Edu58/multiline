.PHONY: build test race migrate force_migrate fmt sqlc

fmt:
	go fmt ./...

test:
	go test -v -cover ./...

build:
	go build ./...

race:
	go test -race ./...

migrate:
	migrate -database postgresql://postgres:postgres@localhost:5432/multiline_dev?sslmode=disable \
	-path ./db/migrations/ \
	-verbose $(where)

force_migrate:
	migrate -database postgresql://postgres:postgres@localhost:5432/multiline_dev?sslmode=disable \
	-path ./db/migrations/ \
	-verbose force 1

sqlc:
	sqlc generate
