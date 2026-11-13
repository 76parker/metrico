package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/76Parker/metrico/internal/agent/reporter"
	"github.com/76Parker/metrico/internal/jsonconfig"
	"testing"
	"time"
)

func TestReporterError(t *testing.T) {
	if err := reporterError(fmt.Errorf("wrapped: %w", context.Canceled)); err != nil {
		t.Fatalf("wrapped cancellation: %v", err)
	}
	sendErr := &reporter.SendError{Err: context.Canceled}
	if err := reporterError(sendErr); !errors.Is(err, sendErr) {
		t.Fatalf("send error = %v, want %v", err, sendErr)
	}
}

func TestResolveRateLimit_Default(t *testing.T) {
	got, err := resolveRateLimit(0, false, 0, false)
	if err != nil || got != 1 {
		t.Fatalf("rate limit = %d, want 1", got)
	}
}

func TestResolveRateLimit_Environment(t *testing.T) {
	got, err := resolveRateLimit(0, false, 4, true)
	if err != nil || got != 4 {
		t.Fatalf("rate limit = %d, want 4", got)
	}
}

func TestResolveRateLimit_FlagTakesPrecedence(t *testing.T) {
	got, err := resolveRateLimit(3, true, 4, true)
	if err != nil || got != 3 {
		t.Fatalf("rate limit = %d, want 3", got)
	}
}

func TestResolveRateLimit_RejectsNonPositive(t *testing.T) {
	if _, err := resolveRateLimit(0, false, 0, true); err == nil {
		t.Fatal("expected zero rate limit to be rejected")
	}
}

func TestEnvConfig_RedefineConfigFromEnvKey(t *testing.T) {
	previousKey := key
	t.Cleanup(func() {
		key = previousKey
	})
	key = "from-flag"
	t.Setenv("KEY", "from-environment")

	(&envConfig{Key: "from-environment"}).redefineConfigFromEnv()

	if key != "from-environment" {
		t.Fatalf("key = %q, want %q", key, "from-environment")
	}
}

func TestJSONConfigAndOverrides(t *testing.T) {
	oldAddr, oldKey, oldCrypto, oldPoll, oldReport, oldRate := addr, key, cryptoKeyPath, pollInterval, reportInterval, rateLimit
	t.Cleanup(func() {
		addr, key, cryptoKeyPath, pollInterval, reportInterval, rateLimit = oldAddr, oldKey, oldCrypto, oldPoll, oldReport, oldRate
	})
	addr, key, cryptoKeyPath, pollInterval, reportInterval, rateLimit = "flag-address", "old", "old", time.Second, time.Second, 1
	cfg := jsonconfig.Agent{Address: ptr("file-address"), Key: ptr("file-key"), CryptoKey: ptr("public.pem"), PollInterval: ptr(jsonconfig.Duration(500 * time.Millisecond)), ReportInterval: ptr(jsonconfig.Duration(1500 * time.Millisecond)), RateLimit: ptr(4)}
	applyJSONConfig(cfg, map[string]bool{"a": true})
	if addr != "flag-address" || key != "file-key" || cryptoKeyPath != "public.pem" || pollInterval != 500*time.Millisecond || reportInterval != 1500*time.Millisecond || rateLimit != 4 {
		t.Fatalf("JSON settings not applied: %s %s %s %s %s %d", addr, key, cryptoKeyPath, pollInterval, reportInterval, rateLimit)
	}
	t.Setenv("KEY", "")
	t.Setenv("POLL_INTERVAL", "3")
	(&envConfig{Key: "", PollInterval: 3}).redefineConfigFromEnv()
	if key != "" || pollInterval != 3*time.Second {
		t.Fatal("environment did not override JSON")
	}
}
func ptr[T any](v T) *T { return &v }
