package main

import (
	"context"
	"crypto/rsa"
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
	"github.com/76Parker/metrico/internal/cryptomsg"
	"github.com/76Parker/metrico/internal/jsonconfig"
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
	cryptoKeyPath  string
)

type envConfig struct {
	CryptoKey      string `env:"CRYPTO_KEY"`
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
	if _, ok := os.LookupEnv("CRYPTO_KEY"); ok {
		cryptoKeyPath = e.CryptoKey
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	var pollSeconds, reportSeconds int
	var configPath string
	jsonconfig.RegisterPath(flag.CommandLine, &configPath)
	flag.StringVar(&addr, "a", defaultAddr, "Listener address")
	flag.StringVar(&key, "k", "", "Hash key")
	flag.StringVar(&cryptoKeyPath, "crypto-key", "", "RSA public key file")
	flag.IntVar(&pollSeconds, "p", int(defaultPollInterval/time.Second), "Poll interval for metric provider in seconds")
	flag.IntVar(&reportSeconds, "r", int(defaultReportInterval/time.Second), "Report interval for metric reporter in seconds")
	flag.IntVar(&rateLimit, "l", defaultRateLimit, "Maximum concurrent outgoing requests")
	flag.Parse()

	pollInterval = time.Duration(pollSeconds) * time.Second
	reportInterval = time.Duration(reportSeconds) * time.Second
	visited := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	configPath, pathErr := jsonconfig.ResolvePath(flag.CommandLine, configPath)
	if pathErr != nil {
		log.Fatal(pathErr)
	}
	if configPath != "" {
		var fileCfg jsonconfig.Agent
		if err := jsonconfig.Load(configPath, &fileCfg); err != nil {
			log.Fatal(err)
		}
		applyJSONConfig(fileCfg, visited)
	}

	envCfg, err := applyEnvOverrides()
	if err != nil {
		log.Fatalf("parse config from env error: %v", err)
	}
	envCfg.redefineConfigFromEnv()
	flagRateLimitSet := visited["l"]
	_, rateLimitEnvSet := os.LookupEnv("RATE_LIMIT")
	if flagRateLimitSet || rateLimitEnvSet {
		rateLimit, err = resolveRateLimit(rateLimit, flagRateLimitSet, envCfg.RateLimit, rateLimitEnvSet)
	} else if rateLimit <= 0 {
		err = fmt.Errorf("rate limit must be positive")
	}
	if err != nil {
		log.Fatal(err)
	}

	var publicKey *rsa.PublicKey
	if cryptoKeyPath != "" {
		publicKey, err = cryptomsg.LoadPublic(cryptoKeyPath)
		if err != nil {
			log.Fatalf("load public key: %v", err)
		}
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
		reporter.WithCryptoKey(publicKey),
		reporter.WithRateLimit(rateLimit),
		reporter.WithSystemMetricProvider(systemProvider),
	)
	if err := reporterError(reporter.Run(ctx)); err != nil {
		log.Fatal(err)
	}
}

func reporterError(err error) error {
	var sendErr *reporter.SendError
	if errors.As(err, &sendErr) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func applyEnvOverrides() (envConfig, error) {
	return env.ParseAs[envConfig]()
}

func applyJSONConfig(c jsonconfig.Agent, flags map[string]bool) {
	if c.Address != nil && !flags["a"] {
		addr = *c.Address
	}
	if c.Key != nil && !flags["k"] {
		key = *c.Key
	}
	if c.CryptoKey != nil && !flags["crypto-key"] {
		cryptoKeyPath = *c.CryptoKey
	}
	if c.PollInterval != nil && !flags["p"] {
		pollInterval = time.Duration(*c.PollInterval)
	}
	if c.ReportInterval != nil && !flags["r"] {
		reportInterval = time.Duration(*c.ReportInterval)
	}
	if c.RateLimit != nil && !flags["l"] {
		rateLimit = *c.RateLimit
	}
}
