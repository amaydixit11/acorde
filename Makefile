BINARY_NAME=acorde

.PHONY: all build test clean run

all: build

build:
	go build -o $(BINARY_NAME) ./cmd/acorde

test:
	go test ./...

clean:
	go clean
	rm -f $(BINARY_NAME)


release:
	@./scripts/build_release.sh

docker:
	docker build -t acorde .

docker-run:
	docker-compose up -d

run: build
	./$(BINARY_NAME) daemon
