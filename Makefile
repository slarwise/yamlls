.PHONY: install
install:
	go build -o ~/go/bin/yamlls ./cmd/main.go

.PHONY: test
test:
	go test ./...
