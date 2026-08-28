.PHONY: build test vet fmt lint golden update-golden offline ci desktop

build:
	go build ./...

test:
	go test ./... -count=1

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

lint: fmt vet

golden:
	go test ./internal/review/ -run TestGolden -count=1 -update

update-golden:
	go test ./internal/review/ -run TestGolden -count=1 -update

offline:
	./scripts/test-offline.sh

desktop:
	go build -tags desktop -o /tmp/loka ./desktop/

ci: lint test offline build desktop
