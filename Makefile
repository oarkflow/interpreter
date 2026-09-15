SHELL := /bin/sh

VSCODE_EXTENSION_DIR := vscode-extension
VSCODE_EXTENSION_ID := oarkflow.spl-vscode
VSCODE_EXTENSION_VERSION := 0.1.0
VSCODE_EXTENSIONS_DIR ?= $(HOME)/.vscode/extensions
VSCODE_EXTENSION_INSTALL_DIR := $(VSCODE_EXTENSIONS_DIR)/$(VSCODE_EXTENSION_ID)-$(VSCODE_EXTENSION_VERSION)
CODE ?= code
VSCODE_URL_SCHEME ?= vscode
# GO_MODULE_DIRS lists every directory containing its own go.mod in this
# tree (verified via `find . -name go.mod`). Keep this in sync when modules
# are added/removed/renamed.
GO_MODULE_DIRS := . \
	plugins \
	cmd/interpreter \
	cmd/spltool-full \
	examples/app \
	benchmarks/exprcompare

BUILTINDOCS_OUT := docs/reference/builtins.md

.PHONY: test test-all test-race test-spl-corpus vet-all vulncheck release-check install-extension vscode-extension-install reload-vscode vscode-extension-compile vscode-extension-clean builtins-doc builtins-doc-check

# These loops intentionally do NOT use `set -e`: every module is attempted
# even if an earlier one fails, and the failing module list is reported at
# the end with a non-zero exit so CI still fails.
test:
	@failed=""; for module in $(GO_MODULE_DIRS); do \
		echo "Testing $$module"; \
		(cd "$$module" && go test ./...) || failed="$$failed $$module"; \
	done; \
	if [ -n "$$failed" ]; then echo "FAILED modules:$$failed"; exit 1; fi

test-all: test

test-race:
	@failed=""; for module in $(GO_MODULE_DIRS); do \
		echo "Race testing $$module"; \
		(cd "$$module" && go test -race ./...) || failed="$$failed $$module"; \
	done; \
	if [ -n "$$failed" ]; then echo "FAILED modules:$$failed"; exit 1; fi

test-spl-corpus:
	./scripts/test_spl_corpus.sh

vet-all:
	@failed=""; for module in $(GO_MODULE_DIRS); do \
		echo "Vetting $$module"; \
		(cd "$$module" && go vet ./...) || failed="$$failed $$module"; \
	done; \
	if [ -n "$$failed" ]; then echo "FAILED modules:$$failed"; exit 1; fi

vulncheck:
	@failed=""; for module in $(GO_MODULE_DIRS); do \
		echo "Vulncheck $$module"; \
		(cd "$$module" && go run golang.org/x/vuln/cmd/govulncheck@latest ./...) || failed="$$failed $$module"; \
	done; \
	if [ -n "$$failed" ]; then echo "FAILED modules:$$failed"; exit 1; fi

# release-check aggregates the checks that should gate a release. It
# currently covers Go modules only (vet + test + the SPL corpus test +
# govulncheck); it intentionally does NOT build/package the VS Code
# extension - wiring that in (and any packaging/signing steps) is left as
# a follow-up.
release-check: vet-all test-all test-spl-corpus vulncheck

# builtins-doc regenerates docs/reference/builtins.md (the SPL builtin
# function reference) from the actual registered builtin set. The
# generator lives in the cmd/spltool-full module (not the root module)
# because it needs the plugins package linked in to see eval.PluginBuiltins
# fully populated; see cmd/spltool-full/cmd/builtindocs/main.go.
builtins-doc:
	@cd cmd/spltool-full && go run ./cmd/builtindocs -out "../../$(BUILTINDOCS_OUT)"

# builtins-doc-check regenerates the reference into a temp file and diffs
# it against the committed docs/reference/builtins.md, failing (non-zero
# exit) if they differ. This is not wired into release-check yet - that is
# a deliberate follow-up decision, not an oversight.
builtins-doc-check:
	@tmp=$$(mktemp) && \
	(cd cmd/spltool-full && go run ./cmd/builtindocs -out "$$tmp") && \
	if diff -u "$(BUILTINDOCS_OUT)" "$$tmp" > /tmp/builtins-doc-check.diff 2>&1; then \
		rm -f "$$tmp" /tmp/builtins-doc-check.diff; \
		echo "builtins-doc-check: OK ($(BUILTINDOCS_OUT) is up to date)"; \
	else \
		cat /tmp/builtins-doc-check.diff; \
		rm -f "$$tmp" /tmp/builtins-doc-check.diff; \
		echo "builtins-doc-check: FAILED - $(BUILTINDOCS_OUT) is out of date; run 'make builtins-doc'"; \
		exit 1; \
	fi

install-extension: vscode-extension-install reload-vscode

vscode-extension-install: vscode-extension-compile
	@echo "Installing SPL VS Code extension to $(VSCODE_EXTENSION_INSTALL_DIR)"
	@rm -rf "$(VSCODE_EXTENSION_INSTALL_DIR)"
	@mkdir -p "$(VSCODE_EXTENSION_INSTALL_DIR)"
	@cp -R \
		"$(VSCODE_EXTENSION_DIR)/package.json" \
		"$(VSCODE_EXTENSION_DIR)/package-lock.json" \
		"$(VSCODE_EXTENSION_DIR)/README.md" \
		"$(VSCODE_EXTENSION_DIR)/language-configuration.json" \
		"$(VSCODE_EXTENSION_DIR)/syntaxes" \
		"$(VSCODE_EXTENSION_DIR)/out" \
		"$(VSCODE_EXTENSION_DIR)/node_modules" \
		"$(VSCODE_EXTENSION_INSTALL_DIR)/"
	@echo "Installed $(VSCODE_EXTENSION_ID). Reloading VS Code window."

vscode-extension-compile:
	@cd "$(VSCODE_EXTENSION_DIR)" && npm install
	@cd "$(VSCODE_EXTENSION_DIR)" && npm run compile

reload-vscode:
	@$(CODE) --reuse-window .
	@open "$(VSCODE_URL_SCHEME)://command/workbench.action.reloadWindow"

vscode-extension-clean:
	@rm -rf "$(VSCODE_EXTENSION_DIR)/node_modules" "$(VSCODE_EXTENSION_DIR)/out"
