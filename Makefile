BIN := bin/gascoigne
PREFIX ?= $(HOME)/.local

.PHONY: build install uninstall test vet fmt fmt-check imports cover eval eval-experimental check clean

build:
	go build -o $(BIN) ./cmd/gascoigne

# Installs for every shell and agent; a running server from another build is replaced on next use.
install:
	CGO_ENABLED=0 go build -trimpath -o $(PREFIX)/bin/gascoigne ./cmd/gascoigne
	@case ":$$PATH:" in *":$(PREFIX)/bin:"*) ;; *) echo "note: add $(PREFIX)/bin to PATH";; esac

uninstall:
	rm -f $(PREFIX)/bin/gascoigne

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

# Needs both prose models and ONNX Runtime (gascoigne prose install -name kompress-v2-base, then
# -name ctxprune-small); not part of check. Each run names its model, so the setup choice does not matter.
eval-experimental: build
	GASCOIGNE_EXPERIMENTAL=prose bin/gascoigne eval -cases testdata/eval-experimental/cases.yaml -record -set optimizers.prose.enabled=true -set optimizers.prose.model=kompress-v2-base
	GASCOIGNE_EXPERIMENTAL=prose bin/gascoigne eval -cases testdata/eval-experimental/ctxprune-small/cases.yaml -record -set optimizers.prose.enabled=true -set optimizers.prose.model=ctxprune-small

check: fmt-check vet imports test

clean:
	rm -rf bin coverage.out
