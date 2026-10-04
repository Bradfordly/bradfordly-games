.PHONY: build panel gateway test cover bdd clean

build: panel gateway

panel:
	go build -o bin/panel ./cmd/panel

gateway:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out -coverpkg=./internal/panel ./internal/panel
	go tool cover -func=coverage.out

bdd:
	go test -v ./internal/panel -run TestFeatures

clean:
	rm -rf bin coverage.out
