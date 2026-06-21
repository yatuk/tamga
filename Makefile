# Tamga — top-level Makefile
# Wraps the common Go + JS workflows so contributors have a single
# entry point for dev, test, load and supply-chain audit tasks.

PROXY_DIR     := proxy
DASHBOARD_DIR := dashboard
K6_IMAGE      := grafana/k6:latest
LOADTEST_URL  ?= http://localhost:8443
LOADTEST_VUS  ?= 25
LOADTEST_DUR  ?= 30s

.PHONY: help build test redteam redteam-report dashboard-build dashboard-test \
        test-load vuln lint clean tidy sidecar-test

help:
	@echo "Tamga Makefile targets:"
	@echo "  build             Build the Go proxy binary (./bin/tamga)"
	@echo "  test              Run all Go unit tests with race detector"
	@echo "  redteam           Run the red-team prompt runner"
	@echo "  dashboard-build   Build the Next.js dashboard"
	@echo "  dashboard-test    Lint + typecheck the dashboard"
	@echo "  test-load         Run k6 load test against LOADTEST_URL"
	@echo "  vuln              govulncheck + npm audit (production deps)"
	@echo "  lint              go vet + eslint"
	@echo "  tidy              go mod tidy + npm install"
	@echo "  clean             Remove build artefacts"

build:
	cd $(PROXY_DIR) && go build -o ../bin/tamga ./cmd/tamga

test:
	cd $(PROXY_DIR) && go test -race ./...

redteam:
	cd $(PROXY_DIR) && go run ./cmd/redteam

# Regenerate the public benchmark artefact shipped under docs/benchmarks/.
# Run this before releasing a new scanner change so the published numbers
# track what CI enforces.
redteam-report:
	cd $(PROXY_DIR) && go run ./cmd/redteam \
		-in ./testdata/redteam/prompts.csv \
		-json ../docs/benchmarks/redteam_latest.json \
		-min-precision 0 -min-recall 0

# Run the Shadow ML sidecar test suite in stub mode (no transformers,
# no torch). Mirrors what .github/workflows/sidecar-ci.yml runs.
sidecar-test:
	cd sidecar && TAMGA_SIDECAR_STUB=1 python -m pytest -q

dashboard-build:
	cd $(DASHBOARD_DIR) && npm run build

dashboard-test:
	cd $(DASHBOARD_DIR) && npm run lint

test-load:
	@echo "Running k6 against $(LOADTEST_URL) ($(LOADTEST_VUS) VUs for $(LOADTEST_DUR))"
	k6 run -e URL=$(LOADTEST_URL) -e VUS=$(LOADTEST_VUS) -e DURATION=$(LOADTEST_DUR) \
		scripts/loadtest.js

vuln:
	@echo "→ go govulncheck"
	cd $(PROXY_DIR) && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@echo "→ npm audit (production)"
	cd $(DASHBOARD_DIR) && npm audit --production || true

lint:
	cd $(PROXY_DIR) && go vet ./...
	cd $(DASHBOARD_DIR) && npm run lint

tidy:
	cd $(PROXY_DIR) && go mod tidy
	cd $(DASHBOARD_DIR) && npm install

clean:
	rm -rf bin $(PROXY_DIR)/bin $(DASHBOARD_DIR)/.next
