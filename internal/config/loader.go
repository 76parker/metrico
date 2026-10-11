// Пакет содержит в себе загрузчик конфигурации
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HttpConfig            HTTP `yaml:"http"`
	SnapshotServiceConfig SnapshotService
	Postgres              Postgres
	Audit                 Audit
}

func Load(configPath string) (*Config, error) {
	var cfg Config
	rawCfg, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if err := yaml.Unmarshal(rawCfg, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &cfg, nil
}
