package main

import (
	"flag"
	"os"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/require"

	"github.com/76Parker/metrico/internal/config"
)

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

func TestEnvConfig_ApplyOverridesKey(t *testing.T) {
	previousKey := key
	t.Cleanup(func() {
		key = previousKey
	})
	key = "from-flag"
	t.Setenv("KEY", "from-environment")

	(&envConfig{Key: "from-environment"}).applyOverrides()

	if key != "from-environment" {
		t.Fatalf("key = %q, want %q", key, "from-environment")
	}
}

func TestDiagnosticsConfiguration_FlagsAndEnvironment(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		args        []string
		environment *string
		expected    string
	}{
		{name: "disabled by default"},
		{name: "flag", args: []string{"-pprof-address", "127.0.0.1:6060"}, expected: "127.0.0.1:6060"},
		{name: "environment", environment: new("[::1]:6061"), expected: "[::1]:6061"},
		{name: "environment overrides flag", args: []string{"-pprof-address", "127.0.0.1:6060"}, environment: new("127.0.0.1:6061"), expected: "127.0.0.1:6061"},
		{name: "empty environment disables flag", args: []string{"-pprof-address", "127.0.0.1:6060"}, environment: new("")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("PPROF_ADDRESS", "")
			require.NoError(t, os.Unsetenv("PPROF_ADDRESS"))
			if testCase.environment != nil {
				t.Setenv("PPROF_ADDRESS", *testCase.environment)
			}
			var cfg config.Config
			flags := flag.NewFlagSet("server", flag.ContinueOnError)
			registerDiagnosticsFlags(flags, &cfg)
			require.NoError(t, flags.Parse(testCase.args))
			configuration, err := env.ParseAs[envConfig]()
			require.NoError(t, err)
			configuration.applyDiagnosticsOverrides(&cfg)
			require.Equal(t, testCase.expected, cfg.PPROFAddress)
		})
	}
}

func TestAuditConfiguration_FlagsAndEnvironment(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		environment map[string]string
		expected    config.Audit
	}{
		{name: "disabled by default"},
		{name: "file flag", args: []string{"--audit-file", "audit.jsonl"}, expected: config.Audit{File: "audit.jsonl"}},
		{
			name:     "url flag",
			args:     []string{"--audit-url", "http://example.com/audit"},
			expected: config.Audit{URL: "http://example.com/audit"},
		},
		{
			name:        "environment enables both receivers",
			environment: map[string]string{"AUDIT_FILE": "env.jsonl", "AUDIT_URL": "https://example.com/audit"},
			expected:    config.Audit{File: "env.jsonl", URL: "https://example.com/audit"},
		},
		{
			name:        "environment overrides both flags",
			args:        []string{"--audit-file", "flag.jsonl", "--audit-url", "http://flag.example/audit"},
			environment: map[string]string{"AUDIT_FILE": "env.jsonl", "AUDIT_URL": "https://env.example/audit"},
			expected:    config.Audit{File: "env.jsonl", URL: "https://env.example/audit"},
		},
		{
			name:        "empty environment disables receivers",
			args:        []string{"--audit-file", "flag.jsonl", "--audit-url", "http://example.com/audit"},
			environment: map[string]string{"AUDIT_FILE": "", "AUDIT_URL": ""},
		},
		{
			name:        "overrides are independent",
			args:        []string{"--audit-file", "flag.jsonl", "--audit-url", "http://example.com/audit"},
			environment: map[string]string{"AUDIT_FILE": ""},
			expected:    config.Audit{URL: "http://example.com/audit"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			for _, name := range []string{"AUDIT_FILE", "AUDIT_URL"} {
				t.Setenv(name, "")
				require.NoError(t, os.Unsetenv(name))
			}
			for name, value := range testCase.environment {
				t.Setenv(name, value)
			}
			var auditCfg config.Audit
			flags := flag.NewFlagSet("server", flag.ContinueOnError)
			registerAuditFlags(flags, &auditCfg)
			require.NoError(t, flags.Parse(testCase.args))
			configFromEnv, err := env.ParseAs[envConfig]()
			require.NoError(t, err)
			configFromEnv.applyAuditOverrides(&auditCfg)
			require.Equal(t, testCase.expected, auditCfg)
		})
	}
}
