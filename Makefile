.PHONY: test vet lint fmt

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt
