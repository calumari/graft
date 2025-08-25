.PHONY: generate test

generate:
	@find . -type f -name graft_gen.go -delete
	@go generate ./...

test: generate
	@go test ./...
