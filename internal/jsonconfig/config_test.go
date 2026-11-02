package jsonconfig

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"address":"","restore":false,"store_interval":"500ms","max_header_bytes":0,"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	var cfg Server
	if err := Load(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Address == nil || *cfg.Address != "" || cfg.Restore == nil || *cfg.Restore || cfg.StoreInterval == nil || time.Duration(*cfg.StoreInterval) != 500*time.Millisecond || cfg.MaxHeaderBytes == nil || *cfg.MaxHeaderBytes != 0 {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
	for _, data := range []string{`{`, `{"store_interval":"oops"}`, `{"store_interval":1}`, `{"restore":"true"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		var c Server
		if err := Load(path, &c); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
	if err := Load(filepath.Join(t.TempDir(), "missing"), &cfg); err == nil {
		t.Fatal("accepted missing file")
	}
}

func TestResolvePath(t *testing.T) {
	for _, tc := range []struct {
		args []string
		env  *string
		want string
		fail bool
	}{
		{want: ""}, {args: []string{"-c", "a.json"}, want: "a.json"}, {args: []string{"-config", "b.json"}, want: "b.json"}, {args: []string{"-c", "a.json", "-config", "b.json"}, want: "b.json"}, {args: []string{"-c", "a.json"}, env: pointer("env.json"), want: "env.json"}, {env: pointer(""), fail: true},
	} {
		t.Run(fmt.Sprint(tc.args, tc.want), func(t *testing.T) {
			t.Setenv("CONFIG", "")
			if err := os.Unsetenv("CONFIG"); err != nil {
				t.Fatal(err)
			}
			if tc.env != nil {
				t.Setenv("CONFIG", *tc.env)
			}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			var path string
			RegisterPath(fs, &path)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			got, err := ResolvePath(fs, path)
			if (err != nil) != tc.fail || got != tc.want {
				t.Fatalf("path %q, error %v", got, err)
			}
		})
	}
}
func pointer[T any](v T) *T { return &v }
