fmt:
	@golangci-lint fmt

lint:
	GOOS=linux golangci-lint run
	GOOS=android golangci-lint run
	GOOS=windows golangci-lint run
	GOOS=darwin golangci-lint run
	GOOS=freebsd golangci-lint run

lint_install:
	go install -v github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

test:
	go test ./...
