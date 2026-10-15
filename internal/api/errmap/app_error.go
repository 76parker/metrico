package errmap

// Машиночитаемые коды ошибок для клиентов HTTP API.
const (
	// CodeNotFound обозначает отсутствие запрошенного ресурса.
	CodeNotFound = "not_found"

	// CodeInternal обозначает внутреннюю или неизвестную ошибку приложения.
	CodeInternal = "internal"

	// CodeInvalidMetricType обозначает неподдерживаемый тип метрики.
	CodeInvalidMetricType = "invalid_metric_type"

	// CodeTypeConflict обозначает конфликт типов метрик с одним именем.
	CodeTypeConflict = "metrics_type_conflict"

	// CodeInvalidMetricValue обозначает значение, не соответствующее типу метрики.
	CodeInvalidMetricValue = "invalid_metric_value"

	// CodeEmpty обозначает отсутствие обязательного значения.
	CodeEmpty = "empty"

	// CodeInvalidJSON обозначает ошибку декодирования тела HTTP-запроса.
	CodeInvalidJSON = "invalid_json"

	// CodeInvalidRequest обозначает недопустимые параметры HTTP-запроса.
	CodeInvalidRequest = "invalid_request"
)
