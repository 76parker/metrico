package metrics

// BatchUpdateCommand задаёт упорядоченный пакет команд изменения метрик.
type BatchUpdateCommand = []UpdateCommand

// UpdateCommand задаёт имя, тип и числовое значение изменения в строковом виде.
// Value заменяет gauge или задаёт приращение counter в зависимости от MetricType.
type UpdateCommand struct {
	// Name задаёт непустое имя метрики.
	Name string
	// MetricType принимает значения gauge или counter.
	MetricType string
	// Value содержит число в строковом виде: float64 для gauge или int64 для counter.
	Value string
}
