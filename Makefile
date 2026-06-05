.PHONY: fmt test lint build build-armv6

fmt:
	go fmt ./...

test:
	go test ./...

lint:
	go vet ./...

build:
	go build -o dist/netwatchd ./cmd/netwatchd

build-armv6:
	./scripts/build-armv6.sh
