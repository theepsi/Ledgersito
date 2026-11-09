# Continuous Integration

## Status

Approved and implemented.

## Purpose and scope

Run the repository's existing correctness and quality checks automatically so
changes are verified before they are merged. CI reports failures; it does not
modify the source branch, publish artifacts, deploy, or change any developer
database.

Implement as a GitHub Actions workflow using the repository's declared Go
version and current check configuration. Grant only the permissions needed to
check out code and run the jobs. Do not expose secrets to pull-request jobs.

## Triggers

- Run on pull requests.
- Run on pushes to the `dev` branch, the repository's primary development
  branch.
- Do not run on a schedule or publish/release events in this first version.

## Checks

Run the following checks, failing the workflow if any command fails:

- Verify Go formatting without modifying files.
- Run `go test ./...`.
- Run `go vet ./...`.
- Run `golangci-lint run` using the repository's v2 configuration.
- Run `sqlc generate`, then fail if generation changes tracked generated code.
- Run the non-database checks and PostgreSQL checks as parallel jobs.
- On every pull request, use an ephemeral PostgreSQL service, apply the current
  migrations to a disposable database named `ledger_test`, then run:
  - `go test -tags=integration -count=1 ./internal/adapters/postgres`
  - `go test -tags=integration,initializationcheck -count=1 ./internal/adapters/postgres`
  - `go test -tags=integration,migrationcheck -count=1 -run TestNameMigration ./internal/adapters/postgres`

The database job must use only the CI service database. Initialization checks
may create and remove uniquely named schemas as they do locally. No CI command
may point at development or production data.

## Acceptance criteria

- Pull-request and branch-push changes run the configured checks automatically.
- A failure in formatting, tests, vet, lint, generated-code consistency, or
  database checks marks the workflow failed.
- SQL generation is verified without silently accepting or committing generated
  differences.
- PostgreSQL integration checks run against a fresh disposable CI service and
  never require developer credentials or repository secrets.
- CI does not apply migrations to any persistent or user-managed database.
- The workflow uses least-privilege permissions and performs no deployment,
  release, artifact publication, or source-branch writes.

## Non-goals

- Deploying the API, publishing releases, building container images, or managing
  production services.
- Replacing or changing local test, lint, formatting, migration, or database
  preparation conventions.
- Performance benchmarks, scheduled tests, coverage thresholds, or branch
  protection configuration.
