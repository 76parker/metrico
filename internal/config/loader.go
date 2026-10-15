// Package config описывает настройки сервера и загрузку конфигурации из YAML.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config объединяет настройки HTTP, диагностики, хранилищ и аудита сервера.
// PPROFAddress задаётся флагом или переменной окружения и не загружается из YAML.
type Config struct {
	// PPROFAddress задаёт числовой loopback-адрес диагностики; пустой адрес отключает pprof.
	PPROFAddress string `yaml:"-"`
	// HTTPConfig содержит настройки основного HTTP-сервера.
	HTTPConfig HTTP `yaml:"http"`
	// SnapshotServiceConfig содержит настройки снимков хранилища в памяти.
	SnapshotServiceConfig SnapshotService
	// Postgres содержит настройки PostgreSQL-хранилища.
	Postgres Postgres
	// Audit независимо включает файловый и HTTP-приёмники аудита.
	Audit Audit
}

// Load читает YAML-файл конфигурации по configPath.
// Флаги и переменные окружения применяет вызывающий код после загрузки файла.
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
