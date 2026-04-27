SHELL := /bin/zsh

.PHONY: build start clean-build generate-openapi

build:
	@mkdir -p "$(CURDIR)/.build"
	@if [[ -f "$(CURDIR)/web/package.json" ]]; then \
		echo "building frontend (web/dist)"; \
		cd "$(CURDIR)/web" && npm install && npm run build; \
	fi
	@go build -o "$(CURDIR)/.build/auto-pr" "$(CURDIR)/cmd/auto-pr"
	@go build -o "$(CURDIR)/.build/auto-prd" "$(CURDIR)/cmd/auto-prd"
	@echo "built $(CURDIR)/.build/auto-pr and $(CURDIR)/.build/auto-prd"

start: build
	@"$(CURDIR)/.build/auto-prd"

clean-build:
	@rm -rf "$(CURDIR)/.build"
	@echo "removed $(CURDIR)/.build"

generate-openapi:
	@cd "$(CURDIR)/web" && npm run openapi:lint
	@cd "$(CURDIR)/web" && npm run openapi:generate
	@go generate ./internal/api
