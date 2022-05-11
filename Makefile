.:
	go run ./cli/buildx --name ss-local --path ./cli/ss-local
	go run ./cli/buildx --name ss-server --path ./cli/ss-server

release:
	go run ./cli/buildx --name ss-local --path ./cli/ss-local --release
	go run ./cli/buildx --name ss-server --path ./cli/ss-server --release