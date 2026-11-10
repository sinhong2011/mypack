# mypack build entry points. Requires Go >= 1.27.3 (GOTOOLCHAIN=auto fetches it).
export GOTOOLCHAIN ?= auto
export CGO_ENABLED = 0

VERSION   ?= 1.0.0
DIST      := $(CURDIR)/dist
REGISTRY  ?= file://$(DIST)/registry/registry.json
PUBKEY     = $(shell cat $(DIST)/keys/registry.pub 2>/dev/null)
LICPUB     = $(shell cat $(DIST)/keys/license.pub 2>/dev/null)
LDFLAGS    = -s -w -X github.com/egoist/mygo.production=1 \
             -X mypack.dev/mypack/internal/cli.Version=$(VERSION) \
             -X mypack.dev/mypack/internal/appservice.DefaultRegistryURL=$(REGISTRY) \
             -X mypack.dev/mypack/internal/runtimemanager.trustedKeysB64=$(PUBKEY) \
             -X mypack.dev/mypack/internal/licensing.issuerKeysB64=$(LICPUB)

.PHONY: all test vet runtimes cli gui app dmg run run-fake clean

all: runtimes app

test:
	go test ./...

vet:
	go vet ./... && GOOS=windows go vet ./... && GOOS=linux go vet ./...

runtimes:
	VERSION=$(VERSION) scripts/release-runtimes.sh

cli:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/mypack-cli ./cmd/mypack-cli

gui:
	go build -trimpath -tags mygo_noinspector -ldflags "$(LDFLAGS)" -o bin/mypack ./cmd/mypack
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/mypack-native-host ./cmd/mypack-native-host

app: gui
	scripts/package-app.sh $(VERSION)

dmg: app
	scripts/package-app.sh $(VERSION) dmg

run:
	MYPACK_REGISTRY=$(REGISTRY) MYPACK_DEV_TRUST=1 go run ./cmd/mypack

run-fake:
	MYPACK_FAKE=1 go run ./cmd/mypack

clean:
	rm -rf bin dist/mypack.app dist/mypack-*.dmg
