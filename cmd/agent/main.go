package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/76Parker/metrico/internal/agent/provider"
	"github.com/76Parker/metrico/internal/agent/reporter"
	"github.com/caarlos0/env/v11"
)

const (
	defaultPollInterval   = 2 * time.Second
	defaultReportInterval = 10 * time.Second
	defaultAddr           = "http://localhost:8080"
	defaultRateLimit      = 1
)

var (
	pollInterval   time.Duration
	reportInterval time.Duration
	rateLimit      int
	addr           string
	key            string
)

type envConfig struct {
	ServerAddress  string `env:"ADDRESS"`
	Key            string `env:"KEY"`
	PollInterval   int    `env:"POLL_INTERVAL"`
	ReportInterval int    `env:"REPORT_INTERVAL"`
	RateLimit      int    `env:"RATE_LIMIT"`
}

func (e *envConfig) redefineConfigFromEnv() {
	if _, ok := os.LookupEnv("ADDRESS"); ok {
		addr = e.ServerAddress
	}
	if _, ok := os.LookupEnv("KEY"); ok {
		key = e.Key
	}
	if _, ok := os.LookupEnv("REPORT_INTERVAL"); ok {
		reportInterval = time.Duration(e.ReportInterval) * time.Second
	}
	if _, ok := os.LookupEnv("POLL_INTERVAL"); ok {
		pollInterval = time.Duration(e.PollInterval) * time.Second
	}
}

func resolveRateLimit(flagValue int, flagSet bool, envValue int, envSet bool) (int, error) {
	value := defaultRateLimit
	if envSet {
		value = envValue
	}
	if flagSet {
		value = flagValue
	}
	if value <= 0 {
		return 0, fmt.Errorf("rate limit must be positive")
	}
	return value, nil
}

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var pollSeconds, reportSeconds int
	flag.StringVar(&addr, "a", defaultAddr, "Listener address")
	flag.StringVar(&key, "k", "", "Hash key")
	flag.IntVar(&pollSeconds, "p", int(defaultPollInterval/time.Second), "Poll interval for metric provider in seconds")
	flag.IntVar(&reportSeconds, "r", int(defaultReportInterval/time.Second), "Report interval for metric reporter in seconds")
	flag.IntVar(&rateLimit, "l", defaultRateLimit, "Maximum concurrent outgoing requests")
	flag.Parse()

	pollInterval = time.Duration(pollSeconds) * time.Second
	reportInterval = time.Duration(reportSeconds) * time.Second

	envCfg, err := applyEnvOverrides()
	if err != nil {
		log.Fatalf("parse config from env error: %v", err)
	}
	envCfg.redefineConfigFromEnv()
	flagRateLimitSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "l" {
			flagRateLimitSet = true
		}
	})
	_, rateLimitEnvSet := os.LookupEnv("RATE_LIMIT")
	rateLimit, err = resolveRateLimit(rateLimit, flagRateLimitSet, envCfg.RateLimit, rateLimitEnvSet)
	if err != nil {
		log.Fatal(err)
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	runtimeProvider := provider.NewMetricProviderWithContext(ctx, pollInterval)
	systemProvider := provider.NewSystemMetricProviderWithContext(ctx, pollInterval)
	reporter := reporter.NewMetricReporter(
		addr,
		reporter.WithHTTPClient(httpClient),
		reporter.WithMetricProvider(runtimeProvider),
		reporter.WithReportInterval(reportInterval),
		reporter.WithKey(key),
		reporter.WithRateLimit(rateLimit),
		reporter.WithSystemMetricProvider(systemProvider),
	)
	if err := reporter.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func applyEnvOverrides() (envConfig, error) {
	return env.ParseAs[envConfig]()
}
