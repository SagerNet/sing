fmt:
	gofumpt -l -w .
	gofmt -s -w .
	gci write -s "standard,prefix(github.com/sagernet/),default" .

lint_install:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

lint:
	GOOS=linux golangci-lint run ./...
	GOOS=windows golangci-lint run ./...
	GOOS=darwin golangci-lint run ./...
	GOOS=freebsd golangci-lint run ./...

test:
	go test -v .

update:
	git fetch
	git reset FETCH_HEAD --hard
	git clean -fdx