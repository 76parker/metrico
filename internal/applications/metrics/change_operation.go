package metrics

import "github.com/76Parker/metrico/internal/domain/metrics"

// ChangeKind определяет операцию, которую хранилище должно выполнить над метрикой.
type ChangeKind int

const (
	// ChangeKindInvalid обозначает неизвестную операцию.
	ChangeKindInvalid ChangeKind = iota
	// ChangeKindSetGauge заменяет значение gauge-метрики.
	ChangeKindSetGauge
	// ChangeKindAddCounter прибавляет приращение к counter-метрике.
	ChangeKindAddCounter
)

// Change описывает операцию хранения с закрытыми полями.
// Создавайте операции через NewSetGaugeChange и NewAddCounterChange.
type Change struct {
	name  string
	kind  ChangeKind
	value float64
	delta int64
}

// Kind возвращает вид операции изменения.
func (c Change) Kind() ChangeKind {
	return c.kind
}

// Name возвращает имя изменяемой метрики.
func (c Change) Name() string {
	return c.name
}

// Value возвращает новое значение gauge; используется для ChangeKindSetGauge.
func (c Change) Value() float64 {
	return c.value
}

// Delta возвращает приращение counter; используется для ChangeKindAddCounter.
func (c Change) Delta() int64 {
	return c.delta
}

// MetricType возвращает тип метрики операции или MetricTypeInvalid для неизвестного Kind.
func (c Change) MetricType() metrics.MetricType {
	switch c.kind {
	case ChangeKindSetGauge:
		return metrics.MetricTypeGauge
	case ChangeKindAddCounter:
		return metrics.MetricTypeCounter
	default:
		return metrics.MetricTypeInvalid
	}
}

func newSetGaugeChange(name string, value float64) Change {
	return Change{
		name:  name,
		kind:  ChangeKindSetGauge,
		value: value,
	}
}

// NewSetGaugeChange creates a change that sets a gauge value.
func NewSetGaugeChange(name string, value float64) Change {
	return newSetGaugeChange(name, value)
}

func newAddCounterChange(name string, delta int64) Change {
	return Change{
		name:  name,
		kind:  ChangeKindAddCounter,
		delta: delta,
	}
}

// NewAddCounterChange создаёт операцию прибавления delta к counter-метрике.
func NewAddCounterChange(name string, delta int64) Change {
	return newAddCounterChange(name, delta)
}
