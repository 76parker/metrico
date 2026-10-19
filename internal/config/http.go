package config

import "time"

// HTTP задаёт адрес, ключ подписи и ограничения основного HTTP-сервера.
type HTTP struct {
	Address           string        `yaml:"address"`
	HashKey           string        `yaml:"hash_key"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
}

// Postgres задаёт строку подключения к PostgreSQL; пустой DSN выбирает хранилище в памяти.
type Postgres struct {
	DSN string
}

// SnapshotService управляет сохранением и восстановлением метрик в файловом режиме.
// Нулевой StoreInterval включает синхронное сохранение после каждого изменения.
type SnapshotService struct {
	StoreInterval   time.Duration
	FileStoragePath string
	Restore         bool
}
