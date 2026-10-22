package main

import "testing"

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
