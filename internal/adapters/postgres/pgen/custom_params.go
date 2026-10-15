// Package pgen содержит sqlc-запросы и параметры доступа к таблице метрик PostgreSQL.
package pgen

// BatchUpsertMetric описывает элемент JSON-батча для запроса BatchUpsert.
type BatchUpsertMetric struct {
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Value *float64 `json:"value"`
	Delta *int64   `json:"delta"`
}

// BatchUpsertParam задаёт пакет метрик для BatchUpsert.
// Перед выполнением запроса сериализуйте пакет в JSON-байты.
type BatchUpsertParam = []BatchUpsertMetric
