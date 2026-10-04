.PHONY: build panel gateway test cover bdd clean

build: panel gateway

panel:
	go build -o bin/panel ./cmd/panel

gateway:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...
	python3 -m unittest scripts.test_backups

cover:
	go test ./internal/backup -covermode=atomic -coverprofile=cover.out
	go tool cover -func=cover.out

bdd:
	go test ./internal/backup -run TestFeatures -v

clean:
	rm -rf bin cover.out
