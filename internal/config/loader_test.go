package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_ReadsYAMLAndReportsFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("http:\n  address: 127.0.0.1:8080\n  read_timeout: 3s\nsnapshotserviceconfig:\n  storeinterval: 1s\n  restore: true\naudit:\n  file: /tmp/events.jsonl\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.HTTPConfig.Address != "127.0.0.1:8080" || config.HTTPConfig.ReadTimeout != 3*time.Second {
		t.Errorf("HTTP config = %+v", config.HTTPConfig)
	}
	if config.SnapshotServiceConfig.StoreInterval != time.Second || !config.SnapshotServiceConfig.Restore {
		t.Errorf("snapshot config = %+v", config.SnapshotServiceConfig)
	}
	if config.Audit.File != "/tmp/events.jsonl" {
		t.Errorf("audit file = %q", config.Audit.File)
	}

	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("Load missing file succeeded")
	}
	if err := os.WriteFile(path, []byte("http: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load malformed YAML succeeded")
	}
}
