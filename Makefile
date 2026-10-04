.PHONY: build panel gateway test cover bdd clean

build: panel gateway

panel:
	go build -o bin/panel ./cmd/panel

gateway:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

cover:
	go test ./internal/adapter ./internal/gateway ./cmd/gateway -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out

bdd:
	go test ./internal/gateway -run TestBDD -count=1

clean:
	rm -rf bin coverage.out
