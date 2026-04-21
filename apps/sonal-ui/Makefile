OPENAPI_SPEC := ../../runtime/internal/agentapi/openapi.yaml
OPENAPI_OUT := src/lib/agentapi/agentapi.generated.ts
OPENAPI_TS := $(CURDIR)/node_modules/.bin/openapi-typescript

.PHONY: lint test generate-api check-api

lint:
	npm run lint
	$(MAKE) check-api

test:
	npm run test:run

generate-api:
	"$(OPENAPI_TS)" "$(OPENAPI_SPEC)" -o "$(OPENAPI_OUT)"

check-api:
	"$(OPENAPI_TS)" "$(OPENAPI_SPEC)" -o "$(OPENAPI_OUT)" --check
