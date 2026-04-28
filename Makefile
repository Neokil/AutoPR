SHELL := /bin/zsh

.PHONY: build start clean-build generate-openapi test-e2e test-e2e-build

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

# Build the E2E Docker image (re-runs only when sources change due to layer caching).
test-e2e-build:
	@docker build -f "$(CURDIR)/e2e/Dockerfile" -t autopr-e2e "$(CURDIR)"

# Build the image (if needed) then run the Playwright E2E suite inside Docker.
test-e2e: test-e2e-build
	@docker run --rm autopr-e2e
