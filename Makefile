# Mypack build entry points. Requires Go >= 1.27.3 (GOTOOLCHAIN=auto fetches it).
export GOTOOLCHAIN ?= auto
export CGO_ENABLED = 0

# Local bundle and dev-runtime version. GitHub releases take their version
# from the vX.Y.Z tag; see docs/releasing.md.
VERSION   ?= 1.0.0
DIST      := $(CURDIR)/dist
REGISTRY  ?= file://$(DIST)/registry/registry.json
PUBKEY     = $(shell cat $(DIST)/keys/registry.pub 2>/dev/null)
LDFLAGS    = -s -w -X github.com/egoist/mygo.production=1 \
             -X mypack.dev/mypack/internal/cli.Version=$(VERSION) \
             -X mypack.dev/mypack/internal/desktop/app.Version=$(VERSION) \
             -X mypack.dev/mypack/internal/appservice.DefaultRegistryURL=$(REGISTRY) \
             -X mypack.dev/mypack/internal/runtimemanager.trustedKeysB64=$(PUBKEY)

.PHONY: all test vet runtimes cli gui app dmg release installers run run-fake clean

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

# make release SIGN_IDENTITY="Developer ID Application: …" NOTARY_PROFILE=mypack
release: gui
	SIGN_IDENTITY="$(SIGN_IDENTITY)" NOTARY_PROFILE="$(NOTARY_PROFILE)" scripts/package-app.sh $(VERSION) dmg

# Installers with mygo build (mygo.json): a .dmg on macOS, a setup .exe
# (needs NSIS) and .deb/.rpm packages. VERSION here is the release's, with
# or without a v; build/ holds the results. The Release workflow runs this.
# UPLOAD=1 also uploads them, with the update files, install.sh,
# install.ps1 and SHA256SUMS, to the GitHub Release vVERSION (mygo build
# -upload: needs gh, GH_TOKEN and MYGO_UPDATER_PRIVATE_KEY).
#   make installers PLATFORM=darwin/universal VERSION=v0.2.0
PLATFORM ?= $(shell go env GOOS)/$(shell go env GOARCH)
MYGO      = go run github.com/egoist/mygo/cmd/mygo@$(shell go list -m -f '{{.Version}}' github.com/egoist/mygo)
installers:
	mkdir -p build
	go run ./scripts/mkicns assets/AppIcon-macOS-1024.png build/AppIcon.icns
	cp mygo.json build/mygo.json.orig
	go run ./scripts/setversion mygo.json $(VERSION)
	$(MYGO) build -platform $(PLATFORM)$(if $(UPLOAD), -upload); s=$$?; mv build/mygo.json.orig mygo.json; exit $$s

run:
	MYPACK_REGISTRY=$(REGISTRY) MYPACK_DEV_TRUST=1 go run ./cmd/mypack

run-fake:
	MYPACK_FAKE=1 go run ./cmd/mypack

clean:
	rm -rf bin build dist/Mypack.app dist/Mypack-*.dmg dist/mypack.app dist/mypack-*.dmg
