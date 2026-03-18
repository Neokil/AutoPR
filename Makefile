SHELL := /bin/zsh

.PHONY: build install

build:
	@mkdir -p .build
	@if [ -f web/package.json ]; then \
		echo "building frontend (web/dist)"; \
		cd web && npm install && npm run build; \
	fi
	@go build -o .build/auto-pr ./cmd/auto-pr
	@go build -o .build/auto-prd ./cmd/auto-prd

install: build
	@ZSHRC="$$HOME/.zshrc"; \
	ALIAS_START="# >>> auto-pr alias >>>"; \
	ALIAS_END="# <<< auto-pr alias <<<"; \
	START="# >>> auto-pr build-path >>>"; \
	END="# <<< auto-pr build-path <<<"; \
	TMP="$$(mktemp)"; \
	if [ -f "$$ZSHRC" ]; then \
		awk -v as="$$ALIAS_START" -v ae="$$ALIAS_END" -v ps="$$START" -v pe="$$END" 'BEGIN{skip=0} $$0==as||$$0==ps{skip=1;next} $$0==ae||$$0==pe{skip=0;next} !skip{print}' "$$ZSHRC" > "$$TMP"; \
	else \
		: > "$$TMP"; \
	fi; \
	{ \
		cat "$$TMP"; \
		echo ""; \
		echo "$$START"; \
		echo "export PATH=\"$(CURDIR)/.build:\$$PATH\""; \
		echo "$$END"; \
	} > "$$ZSHRC"; \
	rm -f "$$TMP"; \
	echo "built .build/auto-pr and .build/auto-prd"; \
	echo "updated PATH in $$ZSHRC"; \
	echo "run: source $$ZSHRC"
