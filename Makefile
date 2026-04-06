.PHONY: build test cover lint clean

build:
	CGO_ENABLED=0 go build -o taipan ./

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run

clean:
	rm -f taipan coverage.out
