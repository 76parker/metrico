// Package jsonconfig loads optional agent and server JSON configuration files.
package jsonconfig

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value, err)
	}
	*d = Duration(duration)
	return nil
}

type Agent struct {
	Address        *string   `json:"address"`
	ReportInterval *Duration `json:"report_interval"`
	PollInterval   *Duration `json:"poll_interval"`
	CryptoKey      *string   `json:"crypto_key"`
	Key            *string   `json:"key"`
	RateLimit      *int      `json:"rate_limit"`
}
type Server struct {
	Address           *string   `json:"address"`
	Restore           *bool     `json:"restore"`
	StoreInterval     *Duration `json:"store_interval"`
	StoreFile         *string   `json:"store_file"`
	DatabaseDSN       *string   `json:"database_dsn"`
	CryptoKey         *string   `json:"crypto_key"`
	Key               *string   `json:"key"`
	AuditFile         *string   `json:"audit_file"`
	AuditURL          *string   `json:"audit_url"`
	PprofAddress      *string   `json:"pprof_address"`
	ReadTimeout       *Duration `json:"read_timeout"`
	WriteTimeout      *Duration `json:"write_timeout"`
	ReadHeaderTimeout *Duration `json:"read_header_timeout"`
	MaxHeaderBytes    *int      `json:"max_header_bytes"`
	IdleTimeout       *Duration `json:"idle_timeout"`
}

func Load(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parse JSON config %q: %w", path, err)
	}
	return nil
}

// RegisterPath adds both supported names for the JSON configuration flag.
func RegisterPath(flags *flag.FlagSet, path *string) {
	flags.StringVar(path, "c", "", "JSON configuration file")
	flags.StringVar(path, "config", "", "JSON configuration file")
}

// ResolvePath gives CONFIG precedence over either flag, including an empty value.
func ResolvePath(flags *flag.FlagSet, path string) (string, error) {
	set := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "c" || f.Name == "config" {
			set = true
		}
	})
	if value, ok := os.LookupEnv("CONFIG"); ok {
		path, set = value, true
	}
	if set && path == "" {
		return "", fmt.Errorf("empty JSON config path")
	}
	return path, nil
}
