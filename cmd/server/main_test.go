package main

import (
	"flag"
	"os"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/require"

	"github.com/76Parker/metrico/internal/config"
)

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
