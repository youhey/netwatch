.PHONY: fmt test lint build build-armv6

fmt:
	go fmt ./...

test:
	go test ./...

lint:
	go vet ./...

build:
	go build -o dist/netwatchd ./cmd/netwatchd
	go build -o dist/netwatch-jsonl ./cmd/netwatch-jsonl

build-armv6:
	./scripts/build-armv6.sh
