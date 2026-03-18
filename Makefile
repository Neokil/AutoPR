SHELL := /bin/zsh

.PHONY: build clean-build register-alias unregister-alias init-config remove-config register-service unregister-service install uninstall

build:
	@bash "$(CURDIR)/scripts/build.sh" "$(CURDIR)"

clean-build:
	@bash "$(CURDIR)/scripts/clean_build.sh" "$(CURDIR)"

register-alias:
	@bash "$(CURDIR)/scripts/register_alias.sh" "$(CURDIR)"

unregister-alias:
	@bash "$(CURDIR)/scripts/unregister_alias.sh" "$(CURDIR)"

init-config:
	@bash "$(CURDIR)/scripts/init_config.sh" "$(CURDIR)"

remove-config:
	@bash "$(CURDIR)/scripts/remove_config.sh" "$(CURDIR)"

register-service: build init-config
	@bash "$(CURDIR)/scripts/register_service.sh" "$(CURDIR)"

unregister-service:
	@bash "$(CURDIR)/scripts/unregister_service.sh" "$(CURDIR)"

install: build register-alias init-config register-service
	@echo "install complete"
	@echo "run: source $$HOME/.zshrc"

uninstall: unregister-service unregister-alias remove-config clean-build
	@echo "uninstall complete"
	@echo "run: source $$HOME/.zshrc"
