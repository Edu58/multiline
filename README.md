# multiline (in active development)

A Go library for a distributed job scheduler backed by PostgreSQL and RabbitMQ.
Applications own configuration, connections, worker registration, and process
lifecycle.

```sh
go get github.com/Edu58/multiline
```

## Packages

- `github.com/Edu58/multiline` (package `scheduler`): scheduler and timing wheel.
- `workers`: job handlers, worker registry, and dispatcher.
- `services`: validated job creation and listing.
- `store`: PostgreSQL connection and transaction management.
- `store/sqlc`: generated database models, query parameters, and queries.
- `config`: optional file-based configuration loader; callers can also construct `config.Config` directly.
- `pkg/logger` and `pkg/strings`: existing utility packages.

Database migrations and query sources live in `db/`. Generated SQL code lives in
`store/sqlc`; run `make sqlc` after changing queries or schemas.

## Embedding

See [the compiling usage example](./example_test.go) for wiring the library into
an application. Implement `workers.Handler` for application-specific work,
register it under a handler name, and map job types to that name with
`config.Config.WORKERS`. Use `services.NewJobsService` to create and list jobs
without a transport layer.

Apply the migrations in `db/migrations` before starting the scheduler. The caller
starts the scheduler with a context, cancels that context on
shutdown, and closes its store. There is no bundled executable or HTTP
API.

The scheduler remains partially implemented. Context cancellation requests that
background loops stop; there is currently no API to wait for in-flight handlers
or guarantee a drained jobs on shutdown.

## Development

```sh
make build
make test
make race
```

The normal test suite does not require external services. PostgreSQL integration
tests require a migrated test database:

```sh
MULTILINE_TEST_DSN='postgresql://postgres:postgres@localhost:5432/multiline_test?sslmode=disable' go test -tags=integration ./store
```

## Design

![High-level design](./docs/images/multiline-design.png)

![Architecture](./docs/images/architecture-with-boundaries.png)
