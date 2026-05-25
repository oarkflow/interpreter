SHELL := /bin/sh

VSCODE_EXTENSION_DIR := vscode-extension
VSCODE_EXTENSION_ID := oarkflow.spl-vscode
VSCODE_EXTENSION_VERSION := 0.1.0
VSCODE_EXTENSIONS_DIR ?= $(HOME)/.vscode/extensions
VSCODE_EXTENSION_INSTALL_DIR := $(VSCODE_EXTENSIONS_DIR)/$(VSCODE_EXTENSION_ID)-$(VSCODE_EXTENSION_VERSION)
CODE ?= code
VSCODE_URL_SCHEME ?= vscode

.PHONY: install-extension vscode-extension-install reload-vscode vscode-extension-compile vscode-extension-clean

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
