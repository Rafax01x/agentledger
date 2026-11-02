.PHONY: build test vet clean

# Default: build the binary.
build:
	go build -o agentledger ./cmd/agentledger

# Run the full test suite with the race detector (required every milestone).
test:
	go test ./... -race

# Static checks (required every milestone).
vet:
	go vet ./...

# Remove build artifacts.
clean:
	rm -f agentledger
