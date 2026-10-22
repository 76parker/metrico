package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/76Parker/metrico/internal/app"
	"github.com/76Parker/metrico/internal/buildinfo"
	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/pkg/logger"
)

// Default values for flags
const (
	defaultAddr            = "localhost:8080"
	defaultFileStoragePath = "metrics.json"
	defaultStoreInterval   = 300
	defaultRestore         = false
	defaultDatabaseDSN     = "" // default value == db is not used
)

// Application configuration flags
var (
	buildVersion = buildinfo.Unknown
	buildDate    = buildinfo.Unknown
	buildCommit  = buildinfo.Unknown

	hostPort        string // -a (or ADDRESS env var)
	fileStoragePath string // -f (or FILE_STORAGE_PATH env var)
	storeInterval   int    // -i (or STORE_INTERVAL env var)
	restore         bool   // -r (or RESTORE env var)
	databaseDSN     string // -d (or DATABASE_DSN env var)
	key             string // -k (or KEY env var)
)

type envConfig struct {
	PPROFAddress    string `env:"PPROF_ADDRESS"`
	Addr            string `env:"ADDRESS"`
	Key             string `env:"KEY"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	StoreInterval   int    `env:"STORE_INTERVAL"`
	Restore         bool   `env:"RESTORE"`
	DatabaseDSN     string `env:"DATABASE_DSN"`
	AuditFile       string `env:"AUDIT_FILE"`
	AuditURL        string `env:"AUDIT_URL"`
}

func (c *envConfig) applyOverrides() {
	if _, ok := os.LookupEnv("ADDRESS"); ok {
		hostPort = c.Addr
	}
	if _, ok := os.LookupEnv("KEY"); ok {
		key = c.Key
	}
	if _, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		fileStoragePath = c.FileStoragePath
	}
	if _, ok := os.LookupEnv("STORE_INTERVAL"); ok {
		storeInterval = c.StoreInterval
	}
	if _, ok := os.LookupEnv("RESTORE"); ok {
		restore = c.Restore
	}
	if _, ok := os.LookupEnv("DATABASE_DSN"); ok {
		databaseDSN = c.DatabaseDSN
	}
}

func main() {
	fmt.Print(buildinfo.Format(buildVersion, buildDate, buildCommit))

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGHUP,
		syscall.SIGQUIT,
	)

	defer stop()
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal("error config load:", err)
	}
	// Flag parsing
	flag.StringVar(&hostPort, "a", defaultAddr, "HTTP listener address")               // -a
	flag.StringVar(&key, "k", "", "Hash key")                                          // -k
	flag.StringVar(&fileStoragePath, "f", defaultFileStoragePath, "file storage path") // -f
	flag.IntVar(&storeInterval, "i", defaultStoreInterval, "store interval")           // -i
	flag.BoolVar(&restore, "r", defaultRestore, "restore from file")                   // -r
	flag.StringVar(&databaseDSN, "d", defaultDatabaseDSN, "database DSN")              // -d
	registerAuditFlags(flag.CommandLine, &cfg.Audit)
	registerDiagnosticsFlags(flag.CommandLine, cfg)

	flag.Parse()
	configFromEnv, err := env.ParseAs[envConfig]()
	if err != nil {
		log.Fatal("error parsing env:", err)
	}
	configFromEnv.applyOverrides()
	configFromEnv.applyAuditOverrides(&cfg.Audit)
	configFromEnv.applyDiagnosticsOverrides(cfg)

	if err := validateHostPort(hostPort); err != nil {
		log.Fatalf("invalid server address: %v", err)
	}

	cfg.HTTPConfig.Address = hostPort
	cfg.HTTPConfig.HashKey = key
	cfg.SnapshotServiceConfig = config.SnapshotService{
		StoreInterval:   time.Duration(storeInterval) * time.Second,
		FileStoragePath: fileStoragePath,
		Restore:         restore,
	}
	cfg.Postgres.DSN = databaseDSN
	applicationLog, err := logger.NewZapLogger("info")
	if err != nil {
		log.Fatal("error creating logger:", err)
	}

	if err := app.Run(ctx, *cfg, applicationLog); err != nil {
		log.Fatal("error running application:", err)
	}
}

func registerAuditFlags(flags *flag.FlagSet, auditCfg *config.Audit) {
	flags.StringVar(&auditCfg.File, "audit-file", "", "Audit log file path")
	flags.StringVar(&auditCfg.URL, "audit-url", "", "Audit log HTTP endpoint")
}

func registerDiagnosticsFlags(flags *flag.FlagSet, cfg *config.Config) {
	flags.StringVar(&cfg.PPROFAddress, "pprof-address", "", "Optional numeric loopback pprof listener address")
}

func (c *envConfig) applyDiagnosticsOverrides(cfg *config.Config) {
	if _, hasValue := os.LookupEnv("PPROF_ADDRESS"); hasValue {
		cfg.PPROFAddress = c.PPROFAddress
	}
}

func (c *envConfig) applyAuditOverrides(auditCfg *config.Audit) {
	if _, hasValue := os.LookupEnv("AUDIT_FILE"); hasValue {
		auditCfg.File = c.AuditFile
	}
	if _, hasValue := os.LookupEnv("AUDIT_URL"); hasValue {
		auditCfg.URL = c.AuditURL
	}
}

func validateHostPort(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf(`address must have the format "host:port"`)
	}
	if port == "" {
		return fmt.Errorf("port cannot be empty")
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("port must be a number")
	}
	if host == "" {
		return fmt.Errorf("address cannot be empty")
	}
	return nil
}
