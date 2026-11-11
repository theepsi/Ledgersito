.PHONY: test test-integration generate vet lint lint-openapi fmt

test:
	go test ./...

test-integration:
	go test -tags=integration -count=1 -v ./internal/adapters/postgres

generate:
	sqlc generate

vet:
	go vet ./...

lint:
	golangci-lint run

lint-openapi:
	REDOCLY_TELEMETRY=off npx --yes @redocly/cli@2.77.0 lint --config redocly.yaml docs/openapi.yaml

fmt:
	golangci-lint fmt
