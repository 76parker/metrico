package main

import (
	"flag"
	"os"
	"testing"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/require"

	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/internal/jsonconfig"
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

func TestJSONConfigAndOverrides(t *testing.T) {
	oldAddr, oldPath, oldStore, oldRestore, oldDSN, oldCrypto, oldKey := hostPort, fileStoragePath, storeInterval, restore, databaseDSN, cryptoKeyPath, key
	t.Cleanup(func() {
		hostPort, fileStoragePath, storeInterval, restore, databaseDSN, cryptoKeyPath, key = oldAddr, oldPath, oldStore, oldRestore, oldDSN, oldCrypto, oldKey
	})
	hostPort, fileStoragePath, restore = "flag-address", "old", true
	cfg := config.Config{HTTPConfig: config.HTTP{ReadTimeout: time.Second}}
	interval := time.Second
	file := jsonconfig.Server{Address: ptr("file-address"), Restore: ptr(false), StoreFile: ptr("file.db"), StoreInterval: ptr(jsonconfig.Duration(500 * time.Millisecond)), DatabaseDSN: ptr("dsn"), CryptoKey: ptr("private.pem"), Key: ptr("key"), AuditFile: ptr("audit"), AuditURL: ptr("url"), PprofAddress: ptr("127.0.0.1:6060"), ReadTimeout: ptr(jsonconfig.Duration(2 * time.Second)), WriteTimeout: ptr(jsonconfig.Duration(3 * time.Second)), ReadHeaderTimeout: ptr(jsonconfig.Duration(4 * time.Second)), IdleTimeout: ptr(jsonconfig.Duration(5 * time.Second)), MaxHeaderBytes: ptr(10)}
	applyJSONConfig(file, map[string]bool{"a": true, "audit-url": true}, &cfg, &interval)
	if hostPort != "flag-address" || restore || fileStoragePath != "file.db" || interval != 500*time.Millisecond || databaseDSN != "dsn" || cryptoKeyPath != "private.pem" || key != "key" || cfg.Audit.File != "audit" || cfg.Audit.URL != "" || cfg.PPROFAddress != "127.0.0.1:6060" || cfg.HTTPConfig.ReadTimeout != 2*time.Second || cfg.HTTPConfig.WriteTimeout != 3*time.Second || cfg.HTTPConfig.ReadHeaderTimeout != 4*time.Second || cfg.HTTPConfig.IdleTimeout != 5*time.Second || cfg.HTTPConfig.MaxHeaderBytes != 10 {
		t.Fatalf("unexpected JSON settings: %+v", cfg)
	}
	t.Setenv("FILE_STORAGE_PATH", "legacy")
	t.Setenv("STORE_FILE", "new")
	(&envConfig{FileStoragePath: "legacy", StoreFile: "new"}).applyOverrides()
	if fileStoragePath != "new" {
		t.Fatalf("store path = %q", fileStoragePath)
	}
}

func TestTrustedSubnetConfiguration(t *testing.T) {
	oldAddr, oldPath, oldStore, oldRestore, oldDSN, oldCrypto, oldKey, oldSubnet := hostPort, fileStoragePath, storeInterval, restore, databaseDSN, cryptoKeyPath, key, trustedSubnet
	t.Cleanup(func() {
		hostPort, fileStoragePath, storeInterval, restore, databaseDSN, cryptoKeyPath, key, trustedSubnet = oldAddr, oldPath, oldStore, oldRestore, oldDSN, oldCrypto, oldKey, oldSubnet
	})
	for _, tc := range []struct {
		name string
		file *string
		args []string
		env  *string
		want string
	}{
		{name: "disabled by default"},
		{name: "JSON", file: ptr("10.0.0.0/8"), want: "10.0.0.0/8"},
		{name: "flag overrides JSON", file: ptr("10.0.0.0/8"), args: []string{"-t", "192.168.0.0/16"}, want: "192.168.0.0/16"},
		{name: "empty flag disables JSON", file: ptr("10.0.0.0/8"), args: []string{"-t", ""}},
		{name: "environment overrides flag", file: ptr("10.0.0.0/8"), args: []string{"-t", "192.168.0.0/16"}, env: ptr("2001:db8::/32"), want: "2001:db8::/32"},
		{name: "empty environment disables flag", file: ptr("10.0.0.0/8"), args: []string{"-t", "192.168.0.0/16"}, env: ptr("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TRUSTED_SUBNET", "")
			require.NoError(t, os.Unsetenv("TRUSTED_SUBNET"))
			if tc.env != nil {
				t.Setenv("TRUSTED_SUBNET", *tc.env)
			}
			flags := flag.NewFlagSet("server", flag.ContinueOnError)
			registerTrustedSubnetFlag(flags)
			require.NoError(t, flags.Parse(tc.args))
			visited := map[string]bool{}
			flags.Visit(func(f *flag.Flag) { visited[f.Name] = true })
			interval := time.Second
			applyJSONConfig(jsonconfig.Server{TrustedSubnet: tc.file}, visited, &config.Config{}, &interval)
			configuration, err := env.ParseAs[envConfig]()
			require.NoError(t, err)
			configuration.applyOverrides()
			require.Equal(t, tc.want, trustedSubnet)
		})
	}
}

func TestParseTrustedSubnet(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
		fail  bool
	}{
		{value: ""},
		{value: "192.168.1.17/24", want: "192.168.1.0/24"},
		{value: "2001:db8::1/32", want: "2001:db8::/32"},
		{value: "invalid", fail: true},
		{value: "192.168.1.1", fail: true},
		{value: "10.0.0.0/33", fail: true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			subnet, err := parseTrustedSubnet(tc.value)
			if tc.fail {
				require.ErrorContains(t, err, "invalid trusted subnet")
				return
			}
			require.NoError(t, err)
			if tc.want == "" {
				require.Nil(t, subnet)
			} else {
				require.Equal(t, tc.want, subnet.String())
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
