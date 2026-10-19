package main

import "testing"

func TestFormatBuildInfo(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		date     string
		commit   string
		expected string
	}{
		{
			name:     "provided values",
			version:  "v1.2.3",
			date:     "2026-10-19",
			commit:   "abc123",
			expected: "Build version: v1.2.3\nBuild date: 2026-10-19\nBuild commit: abc123\n",
		},
		{
			name:     "all values empty",
			expected: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:     "some values empty",
			version:  "v1.2.3",
			commit:   "abc123",
			expected: "Build version: v1.2.3\nBuild date: N/A\nBuild commit: abc123\n",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := formatBuildInfo(testCase.version, testCase.date, testCase.commit); got != testCase.expected {
				t.Errorf("formatBuildInfo() = %q, want %q", got, testCase.expected)
			}
		})
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
