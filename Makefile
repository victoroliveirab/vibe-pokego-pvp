COMPOSE ?= docker compose
PROD_COMPOSE ?= docker compose -f docker-compose.prod.yml --env-file .env.production

.PHONY: bootstrap compose-config up dev down logs test-smoke prod-compose-config prod-up prod-down prod-logs prod-smoke

bootstrap:
	@if [ ! -f .env ]; then cp .env.example .env; fi
	mkdir -p var testdata/uploads
	@test -d testdata/uploads || (echo "bootstrap failed: testdata/uploads was not created" && exit 1)
	@test -w testdata/uploads || (echo "bootstrap failed: testdata/uploads is not writable" && exit 1)
	touch var/app.db
	@test -f var/app.db || (echo "bootstrap failed: var/app.db was not created" && exit 1)
	npm --prefix frontend install
	cd web && go mod download
	cd worker && go mod download

compose-config:
	$(COMPOSE) config >/dev/null

up:
	$(MAKE) compose-config
	$(COMPOSE) up --build -d
	$(MAKE) test-smoke

dev:
	$(MAKE) compose-config
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down --remove-orphans

logs:
	$(COMPOSE) logs -f --tail=200

test-smoke:
	./scripts/smoke/verify_stack.sh
	./scripts/smoke/e2e.sh

prod-compose-config:
	$(PROD_COMPOSE) config >/dev/null

prod-up:
	@test -f .env.production || (echo "missing .env.production; copy .env.production.example and fill in production secrets" && exit 1)
	$(MAKE) prod-compose-config
	$(PROD_COMPOSE) up --build -d
	$(MAKE) prod-smoke

prod-down:
	$(PROD_COMPOSE) down --remove-orphans

prod-logs:
	$(PROD_COMPOSE) logs -f --tail=200

prod-smoke:
	ENV_FILE=.env.production COMPOSE_CMD="$(PROD_COMPOSE)" ./scripts/smoke/verify_stack.sh
