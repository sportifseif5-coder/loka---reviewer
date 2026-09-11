.PHONY: build test vet fmt lint status-source progress-hint golden update-golden offline ci desktop

build:
	go build ./...

test:
	go test ./... -count=1

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

lint: fmt vet status-source

# Enforce that work-status markers live only in docs/PROGRESS.md.
status-source:
	./scripts/check-status-source.sh

# Advisory rot check: reminds the author to tick docs/PROGRESS.md. Never fails.
progress-hint:
	./scripts/progress-hint.sh

golden:
	go test ./internal/review/ -run TestGolden -count=1 -update

update-golden:
	go test ./internal/review/ -run TestGolden -count=1 -update

offline:
	./scripts/test-offline.sh

desktop:
	go build -tags desktop -o /tmp/loka ./desktop/

ci: lint test offline build desktop progress-hint
