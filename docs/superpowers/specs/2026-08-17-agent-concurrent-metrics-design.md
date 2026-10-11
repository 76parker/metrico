# Concurrent Agent Metrics Design

## Goal

Separate metric collection from metric delivery, bound concurrent outbound HTTP requests, and add host memory/CPU gauges.

## Architecture

The runtime provider and a new gopsutil-based system provider collect snapshots in independent goroutines. A reporter scheduler converts snapshots into delivery jobs and places them on a bounded queue. A worker pool consumes jobs; its worker count is the configured rate limit, so no more than that many outbound requests are active at once.

Configuration is read from `-l` and `RATE_LIMIT`, with the flag taking precedence and a default of one worker. The context controls all tickers, queue operations, workers, and shutdown.

## Metrics

The existing runtime gauges and `PollCount` remain unchanged. The system provider adds `TotalMemory` and `FreeMemory` gauges from `gopsutil/mem`, plus one gauge per logical CPU from `gopsutil/cpu.Percent` named `CPUutilization1` through `CPUutilizationN`.

## Error handling and lifecycle

Collection errors are logged and do not stop other collectors. HTTP errors retain the existing retry behavior and are logged by workers. On cancellation, producers stop first, the queue is closed after producers finish, and workers drain queued jobs before returning.

## Verification

Tests cover rate-limit parsing and precedence, system metric naming/count, bounded concurrent delivery, and cancellation without goroutine leaks or blocked sends. Existing agent and repository tests must remain green.
