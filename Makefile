SHELL := /bin/zsh

.PHONY: build register-alias init-config register-service install

build:
	@bash "$(CURDIR)/scripts/build.sh" "$(CURDIR)"

register-alias:
	@bash "$(CURDIR)/scripts/register_alias.sh" "$(CURDIR)"

init-config:
	@bash "$(CURDIR)/scripts/init_config.sh" "$(CURDIR)"

register-service: build init-config
	@bash "$(CURDIR)/scripts/register_service.sh" "$(CURDIR)"

install: build register-alias init-config register-service
	@echo "install complete"
	@echo "run: source $$HOME/.zshrc"
