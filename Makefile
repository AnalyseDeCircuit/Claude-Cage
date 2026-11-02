# Cage control binary and workbench, with Lima built in from third_party/.
#
#   make / make cage   _output/bin/cage, including the workbench and guest assets
#   make help          Cage build, verification and retained upstream commands
#
# Use make app for the macOS bundle; make/cage retains the binary-only contract.

.DEFAULT_GOAL := cage

GO ?= go
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
# Cage's VERSION file owns the default. Upstream release tooling can still
# supply VERSION explicitly when building a historical agent-vm tag.
VERSION ?= $(shell tr -d '\r\n' < VERSION)
LIMA_VERSION := $(shell awk '$$1 == "lima" { print $$4 }' third_party/SOURCES)

# Resolve Go platform settings only for targets that use them. Help, release
# verification and frontend commands do not require a Go installation.
GUEST_ARCH = $(if $(filter amd64,$(GOARCH)),x86_64,$(if $(filter arm64,$(GOARCH)),aarch64,$(GOARCH)))
EXE = $(if $(filter windows,$(GOOS)),.exe)

ASSETS := internal/limaembed/assets
LDFLAGS := -s -w \
	-X github.com/sylvinus/agent-vm/internal/version.Version=$(VERSION) \
	-X github.com/lima-vm/lima/v2/pkg/version.Version=$(LIMA_VERSION)
# yq only edits Lima's YAML here: without its other formats (Lua, HCL, TOML,
# XML...), 10% smaller. Tests run with the same tags.
TAGS := yq_noxml,yq_nolua,yq_noini,yq_nohcl,yq_notoml,yq_noprops,yq_nocsv,yq_nouri,yq_nobase64,yq_nosh,yq_noshell
# cgo on macOS only, where vz and Lima's file watcher need it. On Linux the
# hostagent's DNS server uses Go's resolver (Lima builds it with the
# system's), and the binary cross-compiles with no C compiler.
CGO_ENABLED ?= $(if $(filter darwin,$(GOOS)),1,0)
GO_BUILD = CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -tags $(TAGS) -ldflags "$(LDFLAGS)"

.PHONY: all cage cage-ui help agent-vm install limactl assets test test-go check-windows test-third-party check-third-party fuzz clean
.PHONY: app install-app package-app package-notarized-app verify-release proxy-core licenses check-licenses check-version dev-ui test-ui test-guest test-tools test-app-updates test-headless

HEADLESS_CAGE ?= _output/bin/cage

all: cage

help:
	@printf '%s\n' \
	  'Cage:' \
	  '  make [cage]         Build _output/bin/cage and its workbench assets' \
	  '  make app            Build _output/Cage.app; check Go licenses first' \
	  '  make install-app    Build and install to ~/Applications (replaces old App)' \
	  '  make package-app    Build, sign and notarize release files; does not publish' \
	  '  make package-notarized-app  Resume packaging an already notarized App' \
	  '  make proxy-core     Download and verify the bundled proxy core' \
	  '  make dev-ui         Start the workbench frontend development server' \
	  '' \
	  'Verification:' \
	  '  make test           Run Go tests; see CONTRIBUTING.md for UI and App checks' \
	  '  make test-ui        Lint and test the workbench (npm ci --prefix ui first)' \
	  '  make test-guest     Test guest JavaScript and Python helpers' \
	  '  make test-tools     Test application cleanup tooling' \
	  '  make test-app-updates  Test native and Go application update logic' \
	  '  make test-headless HEADLESS_UPSTREAM=socks5://127.0.0.1:<port>  Real isolated VM acceptance; see CONTRIBUTING.md' \
	  '  make check-version  Check VERSION against frontend package versions' \
	  '  make check-licenses Check generated Go licenses without changing them' \
	  '  make verify-release RELEASE_DIR=<directory>  Verify a downloaded macOS release' \
	  '  make check-third-party  Check vendored sources against upstream + patches' \
	  '  make test-third-party   Test patched dependencies' \
	  '  make fuzz           Run configured fuzzers (FUZZTIME=1m by default)' \
	  '  make clean          Remove build outputs and generated Lima assets' \
	  '' \
	  'License maintenance:' \
	  '  make licenses       Regenerate THIRD_PARTY_LICENSES; review and commit changes' \
	  '  NOTICE is maintained manually; App licenses are collected during the build.' \
	  '' \
	  'Retained upstream tools (not Cage desktop platform support):' \
	  '  make agent-vm       Build the agent-vm command' \
	  '  make install        Install the agent-vm command, not Cage.app' \
	  '  make limactl        Build Lima debugging tools' \
	  '  make check-windows  Run Windows-targeted Go vet checks' \
	  '' \
	  'Repository layout: docs/repository.md'

# Keep the scripts usable directly; platform checks, signing, replacement and
# cleanup stay in their existing owners rather than being repeated here.
app:
	bash scripts/tools/build-app.sh

install-app:
	bash scripts/tools/build-app.sh --install

package-app:
	bash scripts/tools/package-macos.sh

package-notarized-app:
	bash scripts/tools/package-macos.sh --from-notarized-app

verify-release:
	@test -n "$(RELEASE_DIR)" || { echo 'Usage: make verify-release RELEASE_DIR=<directory>' >&2; exit 2; }
	bash scripts/tests/verify-macos-release.sh "$(RELEASE_DIR)"

proxy-core:
	bash scripts/tools/fetch-proxy-core.sh

licenses:
	bash scripts/tools/licenses

check-licenses:
	bash scripts/tools/licenses --check

check-version:
	node scripts/tests/check-version.mjs

dev-ui:
	npm run --prefix ui dev

# Claude Cage ships a separate entry point with a mandatory proxy profile.
cage: cage-ui assets
	$(GO_BUILD) -o _output/bin/cage ./cmd/cage
	$(if $(filter darwin,$(GOOS)),codesign -f --entitlements third_party/lima/vz.entitlements -s - _output/bin/cage)

cage-ui:
	npm --prefix ui ci --no-audit --no-fund
	npm run --prefix ui build

agent-vm: assets
	$(GO_BUILD) -o _output/bin/agent-vm$(EXE) ./cmd/agent-vm
	$(if $(filter darwin,$(GOOS)),codesign -f -v --entitlements third_party/lima/vz.entitlements -s - _output/bin/agent-vm)

# For developers: the binary install puts on the PATH, with its own
# wording, replacement of earlier installs and next steps.
install: agent-vm
	./_output/bin/agent-vm$(EXE) install

limactl:
	$(GO_BUILD) -o _output/bin/limactl$(EXE) github.com/lima-vm/lima/v2/cmd/limactl
	mkdir -p _output/share/lima
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "-s -w" \
		-o _output/share/lima/lima-guestagent.Linux-$(GUEST_ARCH) github.com/lima-vm/lima/v2/cmd/lima-guestagent
	rm -rf _output/share/lima/templates && cp -R third_party/lima/templates _output/share/lima/templates
	$(if $(filter darwin,$(GOOS)),codesign -f -v --entitlements third_party/lima/vz.entitlements -s - _output/bin/limactl)

# The guest agent runs in the Linux guest: built for it, without cgo.
assets:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) $(GO) build -trimpath \
		-ldflags "-s -w -X github.com/lima-vm/lima/v2/pkg/version.Version=$(LIMA_VERSION)" \
		-o $(ASSETS)/lima-guestagent.Linux-$(GUEST_ARCH) github.com/lima-vm/lima/v2/cmd/lima-guestagent
	gzip -9 -n -f $(ASSETS)/lima-guestagent.Linux-$(GUEST_ARCH)
	rm -rf $(ASSETS)/templates && cp -R third_party/lima/templates $(ASSETS)/templates

test: test-go

test-go:
	$(GO) test -tags $(TAGS) . ./cmd/... ./internal/... ./tests

test-ui:
	npm run --prefix ui lint
	npm test --prefix ui

test-guest:
	node --test internal/cage/guest/clipboard.test.cjs internal/cage/guest/browser-audit.test.mjs internal/cage/guest/browser-downloads.test.mjs
	uv run internal/cage/guest/save-download.test.py
	uv run internal/cage/guest/editor-migrate.test.py

test-tools:
	node --test scripts/tests/cleanup-apps.test.mjs

test-app-updates:
	bash scripts/tests/test-app-updates.sh

# Requires a candidate binary with its bundled core and an authorized local
# proxy. The acceptance runner owns its own profile, HOME, project and VM.
test-headless:
	@test -n "$(HEADLESS_UPSTREAM)" || { echo 'Set HEADLESS_UPSTREAM to an authorized local SOCKS5 endpoint; see CONTRIBUTING.md'; exit 2; }
	$(GO) run ./scripts/tests/headless -cage "$(HEADLESS_CAGE)" -upstream "$(HEADLESS_UPSTREAM)" $(if $(HEADLESS_REPORT),-report "$(HEADLESS_REPORT)") $(if $(HEADLESS_IMAGE_CACHE),-image-cache "$(HEADLESS_IMAGE_CACHE)")

# Retained Windows-targeted vet checks. These do not establish that the Cage
# desktop application supports Windows; its current CI runs on macOS.
check-windows:
	GOOS=windows GOARCH=amd64 $(GO) vet -tags $(TAGS) . ./cmd/... ./internal/... ./tests
	GOOS=windows GOARCH=arm64 $(GO) vet -tags $(TAGS) . ./cmd/... ./internal/... ./tests

# From this module, so that each library is built with the others patched.
# Lima's MCP packages and sshocker's command are not part of agent-vm (-e:
# listing them needs sums nothing built here uses; grep keeps them out of
# the tests).
test-third-party:
	$(GO) test -vet=off -race github.com/pkg/sftp/...
	$(GO) test -race github.com/lima-vm/sshocker/pkg/...
	$(GO) test $$($(GO) list -e github.com/lima-vm/lima/v2/pkg/... | grep -v /mcp)
	$(GO) test github.com/containers/gvisor-tap-vsock/pkg/...

check-third-party:
	scripts/tools/third-party-sync all --check

# Each fuzzer for FUZZTIME (their seeds run with the tests). The vendored
# sshocker's through tests/fuzz/go.work, which makes it fuzzable.
FUZZTIME ?= 1m
FUZZ = github.com/lima-vm/sshocker/pkg/reversesshfs github.com/pkg/sftp ./internal/env ./internal/gitguard \
	./internal/mounts ./internal/paths ./internal/runscript ./internal/ui ./internal/vmname
fuzz:
	@set -e; for pkg in $(FUZZ); do \
	  case $$pkg in ./*) work=off ;; *) work=$(CURDIR)/tests/fuzz/go.work ;; esac; \
	  fs=$$(GOWORK=$$work $(GO) test -vet=off -list '^Fuzz' $$pkg) || { echo "$$fs"; exit 1; }; \
	  for f in $$(echo "$$fs" | grep '^Fuzz'); do \
	    echo "== $$pkg $$f"; \
	    GOWORK=$$work $(GO) test -vet=off -run '^$$' -fuzz "^$$f$$" -fuzztime $(FUZZTIME) $$pkg; \
	  done; \
	done

clean:
	rm -rf _output
	find $(ASSETS) -mindepth 1 ! -name README -exec rm -rf {} +
