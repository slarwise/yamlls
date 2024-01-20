.PHONY: test-manually
test-manually: build
	hx test.yaml

.PHONY: build
build:
	go build -o yamlls ./cmd/main.go

.PHONY: test
test:
	go test ./...
