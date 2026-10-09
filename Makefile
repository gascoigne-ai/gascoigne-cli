BIN := bin/gascoigne

.PHONY: build test vet fmt fmt-check imports cover eval check clean

build:
	go build -o $(BIN) ./cmd/gascoigne

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

imports:
	.harness/scripts/check-imports.sh

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

eval: build
	.harness/scripts/eval.sh

check: fmt-check vet imports test

clean:
	rm -rf bin coverage.out
