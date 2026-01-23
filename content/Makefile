# Content Services Makefile
# Provides targets for building, testing, and running integration tests
#
# Usage:
#   make buf          - Generate protobuf code
#   make test         - Run tests with SQLite (default)
#   make testpg       - Run tests with PostgreSQL
#   make testds       - Run tests with Datastore emulator
#   make testrealDS   - Run tests with real Datastore

.PHONY: all buf clean test testpg testds testrealDS testrealDS-force updb downdb dblogs upds downds dslogs testall

# Default target: generate code and run tests
all: buf

# Generate protobuf code
buf:
	cd protos && buf dep update
	cd protos && buf generate
	goimports -w `find gen -name "*.go"` 2>/dev/null || true
	cd gen/go && go build ./...
	cd gen/gorm && go build ./... 2>/dev/null || true
	cd gen/gorm/dal && go build ./... 2>/dev/null || true
	cd gen/datastore && go build ./... 2>/dev/null || true
	cd gen/datastore/dal && go build ./... 2>/dev/null || true
	cd .. && go test ./content/...

# Clean generated files
clean:
	rm -rf gen
	cd protos && rm -f buf.lock

# Run all tests (SQLite by default)
test:
	cd .. && go test ./content/...

# ============================================================================
# PostgreSQL Integration Tests
# ============================================================================

PG_CONTAINER_NAME := content-test-pg
PG_PORT := 5434
PG_USER := postgres
PG_PASSWORD := testpassword
PG_DB := contentdb

# Start PostgreSQL container
updb:
	@echo "Starting PostgreSQL container..."
	@docker run --rm -d \
		--name $(PG_CONTAINER_NAME) \
		-e POSTGRES_USER=$(PG_USER) \
		-e POSTGRES_PASSWORD=$(PG_PASSWORD) \
		-e POSTGRES_DB=$(PG_DB) \
		-p $(PG_PORT):5432 \
		postgres:15-alpine
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@echo ""
	@echo "PostgreSQL is running!"
	@echo "To run tests with PostgreSQL:"
	@echo "  export CONTENT_TEST_PGDB=$(PG_DB)"
	@echo "  export CONTENT_TEST_PGPORT=$(PG_PORT)"
	@echo "  export CONTENT_TEST_PGUSER=$(PG_USER)"
	@echo "  export CONTENT_TEST_PGPASSWORD=$(PG_PASSWORD)"
	@echo "  make testpg"
	@echo ""
	@echo "To stop: make downdb"

# Stop PostgreSQL container
downdb:
	@echo "Stopping PostgreSQL container..."
	@docker stop $(PG_CONTAINER_NAME) 2>/dev/null || echo "Container not running"

# Tail PostgreSQL logs
dblogs:
	@docker logs -f $(PG_CONTAINER_NAME)

# Run tests with PostgreSQL (starts container if not running)
testpg:
	@if ! docker ps --format '{{.Names}}' | grep -q '^$(PG_CONTAINER_NAME)$$'; then \
		echo "Starting PostgreSQL container..."; \
		docker run --rm -d \
			--name $(PG_CONTAINER_NAME) \
			-e POSTGRES_USER=$(PG_USER) \
			-e POSTGRES_PASSWORD=$(PG_PASSWORD) \
			-e POSTGRES_DB=$(PG_DB) \
			-p $(PG_PORT):5432 \
			postgres:15-alpine; \
		sleep 3; \
	fi
	cd .. && \
	CONTENT_TEST_PGDB=$(PG_DB) \
	CONTENT_TEST_PGPORT=$(PG_PORT) \
	CONTENT_TEST_PGUSER=$(PG_USER) \
	CONTENT_TEST_PGPASSWORD=$(PG_PASSWORD) \
	go test -v ./content/tests/gorm/...

# ============================================================================
# Datastore Emulator Integration Tests
# ============================================================================

DS_CONTAINER_NAME := content-test-datastore
DS_PORT := 8082
DS_PROJECT := content-test-project

# Start Datastore emulator
upds:
	@echo "Starting Datastore emulator container..."
	@docker run --rm -d \
		--name $(DS_CONTAINER_NAME) \
		-p $(DS_PORT):8081 \
		gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators \
		gcloud beta emulators datastore start \
			--host-port=0.0.0.0:8081 \
			--project=$(DS_PROJECT) \
			--no-store-on-disk
	@echo "Waiting for Datastore emulator to be ready..."
	@sleep 5
	@echo ""
	@echo "Datastore emulator is running!"
	@echo "To run tests:"
	@echo "  export DATASTORE_EMULATOR_HOST=localhost:$(DS_PORT)"
	@echo "  export DATASTORE_PROJECT_ID=$(DS_PROJECT)"
	@echo "  make testds"
	@echo ""
	@echo "To stop: make downds"

# Stop Datastore emulator
downds:
	@echo "Stopping Datastore emulator container..."
	@docker stop $(DS_CONTAINER_NAME) 2>/dev/null || echo "Container not running"

# Tail Datastore emulator logs
dslogs:
	@docker logs -f $(DS_CONTAINER_NAME)

# Run tests with Datastore emulator (starts container if not running)
testds: downds
	@if ! docker ps --format '{{.Names}}' | grep -q '^$(DS_CONTAINER_NAME)$$'; then \
		echo "Starting Datastore emulator container..."; \
		docker run --rm -d \
			--name $(DS_CONTAINER_NAME) \
			-p $(DS_PORT):8081 \
			gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators \
			gcloud beta emulators datastore start \
				--host-port=0.0.0.0:8081 \
				--project=$(DS_PROJECT) \
				--no-store-on-disk; \
		sleep 5; \
	fi
	cd .. && \
	DATASTORE_EMULATOR_HOST=localhost:$(DS_PORT) \
	DATASTORE_PROJECT_ID=$(DS_PROJECT) \
	go test -v ./content/tests/datastore/...

# ============================================================================
# Real Datastore Integration Tests
# ============================================================================

# Override via command line: make testrealDS DS_REAL_PROJECT=other-project
DS_REAL_PROJECT ?= gappeng
DS_REAL_CREDENTIALS ?= ~/dev-app-data/secrets/gappeng/gappeng-7bb71377bfa2.json
DS_REAL_NAMESPACE ?= content-test

# Run tests against real Google Cloud Datastore
# This requires:
#   - DS_REAL_PROJECT: GCP project ID
#   - DS_REAL_CREDENTIALS: Path to service account JSON (optional, uses ADC if not set)
#   - DS_REAL_NAMESPACE: Namespace for test entities (default: content-test)
testrealDS:
	@if [ -z "$(DS_REAL_PROJECT)" ]; then \
		echo "Error: DS_REAL_PROJECT must be set to your GCP project ID"; \
		echo "Usage: make testrealDS DS_REAL_PROJECT=my-project DS_REAL_CREDENTIALS=~/path/to/creds.json"; \
		exit 1; \
	fi
	@echo "Running tests against real Datastore..."
	@echo "  Project: $(DS_REAL_PROJECT)"
	@echo "  Namespace: $(DS_REAL_NAMESPACE)"
	@if [ -n "$(DS_REAL_CREDENTIALS)" ]; then \
		echo "  Credentials: $(DS_REAL_CREDENTIALS)"; \
	else \
		echo "  Credentials: Application Default Credentials (ADC)"; \
	fi
	@echo ""
	cd .. && \
	DATASTORE_PROJECT_ID=$(DS_REAL_PROJECT) \
	DATASTORE_CREDENTIALS_FILE=$(DS_REAL_CREDENTIALS) \
	DATASTORE_TEST_NAMESPACE=$(DS_REAL_NAMESPACE) \
	go test -v ./content/tests/datastore/...

# Run tests against real Datastore with force-delete-ns flag
# This will delete any existing entities in the test namespace before running tests
testrealDS-force:
	@if [ -z "$(DS_REAL_PROJECT)" ]; then \
		echo "Error: DS_REAL_PROJECT must be set to your GCP project ID"; \
		echo "Usage: make testrealDS-force DS_REAL_PROJECT=my-project DS_REAL_CREDENTIALS=~/path/to/creds.json"; \
		exit 1; \
	fi
	@echo "Running tests against real Datastore (with force delete)..."
	@echo "  Project: $(DS_REAL_PROJECT)"
	@echo "  Namespace: $(DS_REAL_NAMESPACE)"
	@if [ -n "$(DS_REAL_CREDENTIALS)" ]; then \
		echo "  Credentials: $(DS_REAL_CREDENTIALS)"; \
	else \
		echo "  Credentials: Application Default Credentials (ADC)"; \
	fi
	@echo "  Force delete: enabled"
	@echo ""
	cd .. && \
	DATASTORE_PROJECT_ID=$(DS_REAL_PROJECT) \
	DATASTORE_CREDENTIALS_FILE=$(DS_REAL_CREDENTIALS) \
	DATASTORE_TEST_NAMESPACE=$(DS_REAL_NAMESPACE) \
	go test -v ./content/tests/datastore/... -args -force-delete-ns

# Run all integration tests
testall: testpg testds
