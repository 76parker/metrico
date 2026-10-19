clean:
	rm -f coverage.out

.PHONY: staticlint check-coverage

staticlint:
	go run ./cmd/staticlint ./...

check-coverage:
	go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out > coverage-summary.out
	@awk '/^total:/ { found = 1; coverage = $$3 + 0; print "Total coverage: " $$3; if (coverage < 55) { print "Coverage must be at least 55%"; exit 1 } } END { if (!found) exit 1 }' coverage-summary.out

SUPRESS_OUTPUT = > /dev/null

METRICS_SCHEMA = internal/adapters/filestorage/snapshotschema/metrics-snapshot-v1.schema.json
METRICS_PACKAGE = internal/adapters/filestorage/snapshotschema
METRICS_GEN_OUTPUT_FILE = internal/adapters/filestorage/snapshotschema/metrics-snapshot-v1.gen.go

test-coverage:
	@go test -coverprofile=coverage.out ./... $(SUPRESS_OUTPUT)
	@go tool cover -html=coverage.out $(SUPRESS_OUTPUT)
	@make clean

test-total-coverage:
	@go test ./... -coverprofile=coverage.out $(SUPRESS_OUTPUT)
	@go tool cover -func=coverage.out | grep total | awk '{print $$1, $$3}'
	@make clean $(SUPRESS_OUTPUT)

metrics_schema_codegen:
	go-jsonschema -p $(METRICS_PACKAGE) $(METRICS_SCHEMA) > $(METRICS_GEN_OUTPUT_FILE)


gen-mocks-metrics:
	@mockgen -destination=./internal/applications/metrics/mock_repository_test.go \
		-package=metrics \
		-mock_names Repository=MockRepository \
		./internal/applications/metrics \
		Repository

# generate `dbPinger` mock for package `application/health`
gen-mocks-health:
	@mockgen -destination=./internal/applications/health/mock_db_pinger_test.go \
		-package=health \
		-mock_names dbPinger=MockDbPinger \
		./internal/applications/health \
		dbPinger

# generate mocks for package `api/handlers`
gen-mocks-handlers:
	@mockgen -destination=./internal/api/handlers/mock_metrics_application_test.go \
		-package=handlers \
		-mock_names metricsApplication=MockMetricsApplication \
		./internal/api/handlers \
		metricsApplication
