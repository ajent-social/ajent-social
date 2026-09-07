.PHONY: build test run
build:
	CGO_ENABLED=0 go build -trimpath -o bin/ajentd ./cmd/ajentd
	CGO_ENABLED=0 go build -trimpath -o bin/aj ./cmd/aj
test:
	go test ./...
run:
	go run ./cmd/ajentd
