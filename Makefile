SHELL := /bin/zsh

.PHONY: install

install:
	@mkdir -p .build
	@go build -o .build/ai-orchestrator ./cmd/ai-orchestrator
	@ZSHRC="$$HOME/.zshrc"; \
	ALIAS_START="# >>> ai-orchestrator alias >>>"; \
	ALIAS_END="# <<< ai-orchestrator alias <<<"; \
	START="# >>> ai-orchestrator build-path >>>"; \
	END="# <<< ai-orchestrator build-path <<<"; \
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
	echo "built .build/ai-orchestrator"; \
	echo "updated PATH in $$ZSHRC"; \
	echo "run: source $$ZSHRC"
