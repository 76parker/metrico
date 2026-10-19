// Package audit publishes events about successfully received metrics.
package audit

// Event describes the metrics accepted in one HTTP request.
type Event struct {
	// TS содержит время приёма запроса в секундах Unix.
	TS int64 `json:"ts"`
	// Metrics содержит имена метрик, принятых в одном запросе.
	Metrics []string `json:"metrics"`
	// IPAddress содержит адрес отправителя запроса без номера порта.
	IPAddress string `json:"ip_address"`
}
