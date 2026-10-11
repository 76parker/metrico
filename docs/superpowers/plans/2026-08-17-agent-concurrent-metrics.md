# Concurrent Agent Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Run runtime/system collection independently from HTTP delivery while limiting concurrent requests with `-l`/`RATE_LIMIT` and adding gopsutil gauges.

**Architecture:** Providers publish immutable metric snapshots to a reporter queue. The reporter starts one scheduler and a configured number of workers; workers alone call HTTP delivery. Context cancellation stops producers, closes the queue, and lets workers finish queued work.

**Tech Stack:** Go standard library (`context`, `flag`, `sync`, `runtime`), existing reporter, `github.com/shirou/gopsutil/v3` memory/CPU packages, existing JSON/signature/retry code.

## Global Constraints

- Default rate limit is exactly `1`.
- `-l` overrides `RATE_LIMIT`; both must be positive integers.
- No more than `rateLimit` outbound HTTP requests may be in flight.
- Runtime metric names and existing `PollCount` behavior remain compatible.
- System gauges are named `TotalMemory`, `FreeMemory`, and `CPUutilization1` through `CPUutilizationN`.
- Every changed Go file is formatted with `gofmt`; focused tests run before `go test ./...`.

### Task 1: Configuration and dependency

**Files:**
- Modify: `cmd/agent/main.go`
- Modify: `go.mod`, `go.sum`
- Test: `cmd/agent/main_test.go`

**Interfaces:** Add `rateLimit int`, an `envConfig.RateLimit` field tagged `RATE_LIMIT`, and a positive-value validation helper usable by tests and `main`.

- [ ] Write a failing test for env parsing, default one, flag precedence, and rejection of zero/negative limits.
- [ ] Run `go test ./cmd/agent -run RateLimit`; verify it fails because the configuration does not exist.
- [ ] Add `-l`, parse `RATE_LIMIT`, apply env override only when the flag was not explicitly supplied, and reject non-positive values.
- [ ] Add the explicit gopsutil dependency at the pinned current module version and run `go mod tidy`.
- [ ] Run `gofmt -w cmd/agent/main.go cmd/agent/main_test.go` and `go test ./cmd/agent -run RateLimit`; verify pass.

### Task 2: System metric collector

**Files:**
- Create: `internal/agent/provider/system_metric_provider.go`
- Test: `internal/agent/provider/system_metric_provider_test.go`

**Interfaces:** Define `SystemMetricProvider` with `NewSystemMetricProvider(interval time.Duration)`, `Start(ctx context.Context) <-chan []metrics.Metrics`, and a collection helper that returns the named gauges.

- [ ] Write failing tests asserting memory gauges and exactly `runtime.NumCPU()` CPU gauge names.
- [ ] Run `go test ./internal/agent/provider -run System`; verify failure before implementation.
- [ ] Implement a context-owned ticker goroutine using gopsutil memory and per-CPU percentages; emit copied metric slices and log/skip transient collection errors.
- [ ] Run focused provider tests and `go test -race ./internal/agent/provider`.

### Task 3: Reporter worker pool and producer integration

**Files:**
- Modify: `internal/agent/provider/metric_provider.go`
- Modify: `internal/agent/reporter/metric_reporter.go`
- Modify: `cmd/agent/main.go`
- Test: `internal/agent/reporter/metric_reporter_test.go`

**Interfaces:** Extend the reporter construction with `rateLimit` and a metric snapshot/job channel. Keep `sendBatch` as the single HTTP boundary so tests can instrument the client and count in-flight requests.

- [ ] Write failing tests for at-most-N concurrent requests and cancellation while workers are active.
- [ ] Run `go test ./internal/agent/reporter -run 'Worker|Concurrency|Cancel'`; verify failure.
- [ ] Implement a bounded queue, scheduler, worker startup with `sync.WaitGroup`, and context-aware queue sends; workers call the existing retrying `sendBatch`.
- [ ] Make runtime and system producers publish independently; preserve existing runtime conversion and append system gauges to each outgoing batch.
- [ ] Ensure shutdown waits for producers, closes the queue once, waits for workers, and returns `context.Canceled` consistently.
- [ ] Run focused reporter tests with `-race`.

### Task 4: Full verification and documentation

**Files:**
- Modify: `cmd/agent/README.md`, `internal/agent/README.md` if existing usage text needs the new option.

- [ ] Run `gofmt` on all changed Go files and `go test ./...`.
- [ ] Run `go test -race ./internal/agent/... ./cmd/agent`.
- [ ] Run `git diff --check` and inspect the final diff for lifecycle, precedence, and metric-name correctness.
- [ ] Commit the implementation with `feat: separate agent collection and reporting`.
