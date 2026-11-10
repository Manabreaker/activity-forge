.PHONY: build test lint release

build:
	go build -trimpath -o activityforge .

test:
	go test -race ./...

lint:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)
	go vet ./...
	@if command -v goimports >/dev/null 2>&1; then test -z "$$(goimports -l .)" || (goimports -l .; exit 1); fi

release:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o dist/activityforge-darwin-arm64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o dist/activityforge-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o dist/activityforge-linux-arm64 .
