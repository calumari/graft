.PHONY: generate-examples
generate-examples:
	for d in */; do \
		echo "Generating in $$d"; \
		(cd "$$d" && rm -f graft_gen.go && go generate ./...); \
	done

test-examples: generate-examples
	for d in */; do \
		echo "Testing in $$d"; \
		(cd "$$d" && go test ./...); \
	done