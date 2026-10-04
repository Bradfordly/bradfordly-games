.PHONY: build panel gateway test clean

build: panel gateway

panel:
	go build -o bin/panel ./cmd/panel

gateway:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

clean:
	rm -rf bin
