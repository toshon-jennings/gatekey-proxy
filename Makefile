.PHONY: build run stop restart status app dmg run-app stop-app clean help

BIN      := gatekey-proxy
APP_NAME := Gatekey Proxy
APP_DIR  := build/$(APP_NAME).app
DMG_NAME := Gatekey-Proxy.dmg
PORT     := 8181

build: ## Compile the CLI binary into ./$(BIN)
	go build -o $(BIN) .

run: ## Build, then start the CLI proxy in foreground (Ctrl-C to stop)
	go build -o $(BIN) .
	./$(BIN) start

restart: ## Stop any running CLI proxy, then rebuild
	pkill -f "$(BIN) start" 2>/dev/null || true
	$(MAKE) build

stop: ## Stop any running CLI or Desktop proxy
	pkill -f "gatekey-proxy" 2>/dev/null || true

status: ## Show whether the proxy is up
	@if lsof -nP -iTCP:$(PORT) -sTCP:LISTEN >/dev/null 2>&1; then echo "gatekey-proxy is RUNNING on 127.0.0.1:$(PORT)"; else echo "gatekey-proxy is NOT running"; fi

app: ## Build the macOS .app bundle
	@mkdir -p "$(APP_DIR)/Contents/MacOS" "$(APP_DIR)/Contents/Resources"
	@cp desktop/Info.plist "$(APP_DIR)/Contents/Info.plist"
	@cp desktop/assets/AppIcon.icns "$(APP_DIR)/Contents/Resources/AppIcon.icns"
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$(APP_DIR)/Contents/MacOS/$(BIN)" ./cmd/desktop
	@echo "Built $(APP_DIR)"

dmg: app ## Package the macOS .app bundle into a DMG
	@mkdir -p dist/dmg-stage
	@rm -rf dist/dmg-stage/* dist/$(DMG_NAME)
	@cp -R "$(APP_DIR)" dist/dmg-stage/
	@ln -s /Applications dist/dmg-stage/Applications
	hdiutil create -volname "$(APP_NAME)" -srcfolder dist/dmg-stage -ov -format UDZO "dist/$(DMG_NAME)"
	@rm -rf dist/dmg-stage
	@echo "Created dist/$(DMG_NAME)"

run-app: app ## Build and launch the menu bar app
	open "$(APP_DIR)"

stop-app: ## Stop the menu bar app
	pkill -f "$(APP_DIR)/Contents/MacOS/$(BIN)" 2>/dev/null || true

clean: ## Clean build artifacts and dist
	rm -rf build dist $(BIN)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help