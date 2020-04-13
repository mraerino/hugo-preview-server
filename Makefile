.PHONY: deps clean build

deps:
	go mod download

clean:
	rm -rf ./dist/previews

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o dist/preview -tags extended ./handler.go

in-docker:
	docker run -it --rm -v $$PWD/..:/src -v $$HOME/go/pkg:/go/pkg -w /src/previews golang:1.14 make build
