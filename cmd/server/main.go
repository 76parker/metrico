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
	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/internal/cryptomsg"
	"github.com/76Parker/metrico/internal/jsonconfig"
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
	hostPort        string // -a (or ADDRESS env var)
	fileStoragePath string // -f (or FILE_STORAGE_PATH env var)
	storeInterval   int    // -i (or STORE_INTERVAL env var)
	restore         bool   // -r (or RESTORE env var)
	databaseDSN     string // -d (or DATABASE_DSN env var)
	cryptoKeyPath   string
	key             string // -k (or KEY env var)
	trustedSubnet   string // -t (or TRUSTED_SUBNET env var)
)

type envConfig struct {
	CryptoKey       string `env:"CRYPTO_KEY"`
	GRPCAddress     string `env:"GRPC_ADDRESS"`
	PPROFAddress    string `env:"PPROF_ADDRESS"`
	Addr            string `env:"ADDRESS"`
	Key             string `env:"KEY"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	StoreFile       string `env:"STORE_FILE"`
	StoreInterval   int    `env:"STORE_INTERVAL"`
	Restore         bool   `env:"RESTORE"`
	DatabaseDSN     string `env:"DATABASE_DSN"`
	AuditFile       string `env:"AUDIT_FILE"`
	AuditURL        string `env:"AUDIT_URL"`
	TrustedSubnet   string `env:"TRUSTED_SUBNET"`
}

func (c *envConfig) applyOverrides() {
	if _, ok := os.LookupEnv("ADDRESS"); ok {
		hostPort = c.Addr
	}
	if _, ok := os.LookupEnv("CRYPTO_KEY"); ok {
		cryptoKeyPath = c.CryptoKey
	}
	if _, ok := os.LookupEnv("KEY"); ok {
		key = c.Key
	}
	if _, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		fileStoragePath = c.FileStoragePath
	}
	if _, ok := os.LookupEnv("STORE_FILE"); ok {
		fileStoragePath = c.StoreFile
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
	if _, ok := os.LookupEnv("TRUSTED_SUBNET"); ok {
		trustedSubnet = c.TrustedSubnet
	}
}

func main() {
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
	var jsonPath string
	jsonconfig.RegisterPath(flag.CommandLine, &jsonPath)
	flag.StringVar(&hostPort, "a", defaultAddr, "HTTP listener address") // -a
	flag.StringVar(&cryptoKeyPath, "crypto-key", "", "RSA private key file")
	flag.StringVar(&key, "k", "", "Hash key")                                          // -k
	flag.StringVar(&fileStoragePath, "f", defaultFileStoragePath, "file storage path") // -f
	flag.IntVar(&storeInterval, "i", defaultStoreInterval, "store interval")           // -i
	flag.BoolVar(&restore, "r", defaultRestore, "restore from file")                   // -r
	flag.StringVar(&databaseDSN, "d", defaultDatabaseDSN, "database DSN")              // -d
	registerTrustedSubnetFlag(flag.CommandLine)
	registerAuditFlags(flag.CommandLine, &cfg.Audit)
	registerDiagnosticsFlags(flag.CommandLine, cfg)
	registerGRPCFlags(flag.CommandLine, cfg)

	flag.Parse()
	visited := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	jsonPath, pathErr := jsonconfig.ResolvePath(flag.CommandLine, jsonPath)
	if pathErr != nil {
		log.Fatal(pathErr)
	}
	snapshotInterval := time.Duration(storeInterval) * time.Second
	if jsonPath != "" {
		var fileCfg jsonconfig.Server
		if err := jsonconfig.Load(jsonPath, &fileCfg); err != nil {
			log.Fatal(err)
		}
		applyJSONConfig(fileCfg, visited, cfg, &snapshotInterval)
	}
	configFromEnv, err := env.ParseAs[envConfig]()
	if err != nil {
		log.Fatal("error parsing env:", err)
	}
	configFromEnv.applyOverrides()
	if _, ok := os.LookupEnv("STORE_INTERVAL"); ok {
		snapshotInterval = time.Duration(storeInterval) * time.Second
	}
	configFromEnv.applyAuditOverrides(&cfg.Audit)
	configFromEnv.applyDiagnosticsOverrides(cfg)
	configFromEnv.applyGRPCOverrides(cfg)

	if err := validateHostPort(hostPort); err != nil {
		log.Fatalf("invalid server address: %v", err)
	}
	if cfg.GRPCAddress != "" {
		if err := validateHostPort(cfg.GRPCAddress); err != nil {
			log.Fatalf("invalid gRPC server address: %v", err)
		}
	}

	cfg.HTTPConfig.Address = hostPort
	cfg.HTTPConfig.HashKey = key
	cfg.HTTPConfig.TrustedSubnet, err = parseTrustedSubnet(trustedSubnet)
	if err != nil {
		log.Fatal(err)
	}
	if cryptoKeyPath != "" {
		cfg.HTTPConfig.CryptoKey, err = cryptomsg.LoadPrivate(cryptoKeyPath)
		if err != nil {
			log.Fatalf("load private key: %v", err)
		}
	}
	cfg.SnapshotServiceConfig = config.SnapshotService{
		StoreInterval:   snapshotInterval,
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

func registerGRPCFlags(flags *flag.FlagSet, cfg *config.Config) {
	flags.StringVar(&cfg.GRPCAddress, "grpc-address", "", "Optional gRPC listener address")
}

func registerTrustedSubnetFlag(flags *flag.FlagSet) {
	flags.StringVar(&trustedSubnet, "t", "", "Trusted subnet in CIDR notation")
}

func (c *envConfig) applyDiagnosticsOverrides(cfg *config.Config) {
	if _, hasValue := os.LookupEnv("PPROF_ADDRESS"); hasValue {
		cfg.PPROFAddress = c.PPROFAddress
	}
}

func (c *envConfig) applyGRPCOverrides(cfg *config.Config) {
	if _, hasValue := os.LookupEnv("GRPC_ADDRESS"); hasValue {
		cfg.GRPCAddress = c.GRPCAddress
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

func parseTrustedSubnet(value string) (*net.IPNet, error) {
	if value == "" {
		return nil, nil
	}
	_, subnet, err := net.ParseCIDR(value)
	if err != nil {
		return nil, fmt.Errorf("invalid trusted subnet %q: %w", value, err)
	}
	return subnet, nil
}

func applyJSONConfig(c jsonconfig.Server, flags map[string]bool, cfg *config.Config, interval *time.Duration) {
	if c.Address != nil && !flags["a"] {
		hostPort = *c.Address
	}
	if c.GRPCAddress != nil && !flags["grpc-address"] {
		cfg.GRPCAddress = *c.GRPCAddress
	}
	if c.Key != nil && !flags["k"] {
		key = *c.Key
	}
	if c.CryptoKey != nil && !flags["crypto-key"] {
		cryptoKeyPath = *c.CryptoKey
	}
	if c.Restore != nil && !flags["r"] {
		restore = *c.Restore
	}
	if c.StoreFile != nil && !flags["f"] {
		fileStoragePath = *c.StoreFile
	}
	if c.StoreInterval != nil && !flags["i"] {
		*interval = time.Duration(*c.StoreInterval)
	}
	if c.DatabaseDSN != nil && !flags["d"] {
		databaseDSN = *c.DatabaseDSN
	}
	if c.AuditFile != nil && !flags["audit-file"] {
		cfg.Audit.File = *c.AuditFile
	}
	if c.AuditURL != nil && !flags["audit-url"] {
		cfg.Audit.URL = *c.AuditURL
	}
	if c.PprofAddress != nil && !flags["pprof-address"] {
		cfg.PPROFAddress = *c.PprofAddress
	}
	if c.TrustedSubnet != nil && !flags["t"] {
		trustedSubnet = *c.TrustedSubnet
	}
	if c.ReadTimeout != nil {
		cfg.HTTPConfig.ReadTimeout = time.Duration(*c.ReadTimeout)
	}
	if c.WriteTimeout != nil {
		cfg.HTTPConfig.WriteTimeout = time.Duration(*c.WriteTimeout)
	}
	if c.ReadHeaderTimeout != nil {
		cfg.HTTPConfig.ReadHeaderTimeout = time.Duration(*c.ReadHeaderTimeout)
	}
	if c.MaxHeaderBytes != nil {
		cfg.HTTPConfig.MaxHeaderBytes = *c.MaxHeaderBytes
	}
	if c.IdleTimeout != nil {
		cfg.HTTPConfig.IdleTimeout = time.Duration(*c.IdleTimeout)
	}
}
