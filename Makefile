.PHONY: build run test clean

build:
	go build -o bin/server ./cmd/server/

run:
	go run ./cmd/server/

test:
	go test ./internal/... -v

clean:
	rm -rf bin/
