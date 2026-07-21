SHELL := /bin/sh

VSCODE_EXTENSION_DIR := vscode-extension
VSCODE_EXTENSION_ID := oarkflow.spl-vscode
VSCODE_EXTENSION_VERSION := 0.1.0
VSCODE_EXTENSIONS_DIR ?= $(HOME)/.vscode/extensions
VSCODE_EXTENSION_INSTALL_DIR := $(VSCODE_EXTENSIONS_DIR)/$(VSCODE_EXTENSION_ID)-$(VSCODE_EXTENSION_VERSION)
CODE ?= code
VSCODE_URL_SCHEME ?= vscode
GO_MODULE_DIRS := . \
	builtins/cryptoextra \
	builtins/database \
	builtins/images \
	builtins/integrations \
	builtins/tools \
	builtins/xql \
	config/yaml \
	cmd/interpreter \
	benchmarks/exprcompare

.PHONY: test test-all test-race test-spl-corpus vet-all install-extension vscode-extension-install reload-vscode vscode-extension-compile vscode-extension-clean

test:
	go test ./...

test-all:
	@set -e; for module in $(GO_MODULE_DIRS); do \
		echo "Testing $$module"; \
		(cd "$$module" && go test ./...); \
	done

test-race:
	go test -race ./...

test-spl-corpus:
	./scripts/test_spl_corpus.sh

vet-all:
	@set -e; for module in $(GO_MODULE_DIRS); do \
		echo "Vetting $$module"; \
		(cd "$$module" && go vet ./...); \
	done

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
