// Package audit publishes events about successfully received metrics.
package audit

// Event describes the metrics accepted in one HTTP request.
type Event struct {
	TS        int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}
