.PHONY: build run stop restart status help

BIN  := gatekey-proxy
PORT := 8181

build: ## Compile the current source into ./$(BIN)
	go build -o $(BIN) .

run: ## Build, then start the proxy (foreground; Ctrl-C to stop)
	go build -o $(BIN) .
	./$(BIN) start

restart: ## Stop any running copy, then rebuild
	pkill -f "$(BIN) start" 2>/dev/null || true
	$(MAKE) build

stop: ## Stop any running copy
	pkill -f "$(BIN) start" 2>/dev/null || true

status: ## Show whether the proxy is up
	@if lsof -nP -iTCP:$(PORT) -sTCP:LISTEN >/dev/null 2>&1; then echo "gatekey-proxy is RUNNING on 127.0.0.1:$(PORT)"; else echo "gatekey-proxy is NOT running"; fi

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help