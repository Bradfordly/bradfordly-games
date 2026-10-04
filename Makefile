.PHONY: build panel gateway test cover bdd clean

build: panel gateway

panel:
	go build -o bin/panel ./cmd/panel

gateway:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

cover:
	go test ./internal/world ./internal/gameprofile ./cmd/gateway ./cmd/panel -coverprofile=cover.out
	go tool cover -func=cover.out

bdd:
	go test ./tests/bdd -count=1

clean:
	rm -rf bin cover.out
