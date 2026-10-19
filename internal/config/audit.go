package config

// Audit configures the independently enabled audit receivers.
type Audit struct {
	// File задаёт путь JSONL-журнала; пустое значение отключает файловый аудит.
	File string
	// URL задаёт HTTP-приёмник событий; пустое значение отключает отправку по HTTP.
	URL string
}
