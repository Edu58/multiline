.PHONY: server migrate force_migrate fmt

fmt:
	go fmt -n ./...

test:
	go test -v -cover ./...

server:
	go run ./cmd/api/main.go

race:
	go test -race ./...

migrate:
	migrate -database postgresql://postgres:postgres@localhost:5432/multiline_dev?sslmode=disable \
	-path ./internal/db/migrations/ \
	-verbose $(where)

force_migrate:
	migrate -database postgresql://postgres:postgres@localhost:5432/multiline_dev?sslmode=disable \
	-path ./internal/db/migrations/ \
	-verbose force 1

sqlc:
	sqlc generate
