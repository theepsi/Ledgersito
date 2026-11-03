.PHONY: test test-integration generate vet lint fmt

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

fmt:
	golangci-lint fmt
