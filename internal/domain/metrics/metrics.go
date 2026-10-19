// Package metrics определяет типы метрик и ошибки их валидации и хранения.
package metrics

// MetricType определяет способ интерпретации значения метрики.
type MetricType string

const (
	// MetricTypeCounter обозначает целочисленную метрику с накоплением приращений.
	MetricTypeCounter MetricType = "counter"
	// MetricTypeGauge обозначает метрику с заменяемым вещественным значением.
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeInvalid обозначает неизвестный или неподдерживаемый тип метрики.
	MetricTypeInvalid MetricType = "invalid"
)

// GaugeValue представляет вещественное значение gauge-метрики.
type GaugeValue float64

// CounterValue представляет целочисленное значение counter-метрики.
type CounterValue int64

// Metrics описывает метрику в JSON-контракте сервера и агента.
// Для counter задано Delta, для gauge — Value. Указатели отличают отсутствующее
// значение от нуля и позволяют опускать неиспользуемое поле при JSON-кодировании.
type Metrics struct {
	// ID содержит имя метрики.
	ID string `json:"id"`
	// Type определяет используемое поле значения.
	Type MetricType `json:"type"`
	// Delta содержит приращение при записи counter или накопленное значение при чтении.
	Delta *int64 `json:"delta,omitempty"`
	// Value содержит текущее значение gauge.
	Value *float64 `json:"value,omitempty"`
	// Hash содержит необязательную подпись из JSON-представления метрики.
	Hash string `json:"hash,omitempty"`
}
