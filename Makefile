.PHONY: test
test:
	go test -race ./... -count=1

.PHONY: test-cover
test-cover:
	@go test -race ./... -coverprofile unit_test_coverage.out -count=1 && go tool cover -func=unit_test_coverage.out

.PHONY: lint
lint:
	golangci-lint run

.PHONY: tidy
tidy:
	go mod tidy