package middleware

import (
	"time"

	"github.com/76Parker/metrico/pkg/logger"
	"github.com/gin-gonic/gin"
)

// Тип описывающий список полей для логирования
type logFields []any

// WithLogging записывает результат HTTP-запроса, его длительность и имя обработчика.
// Ошибки клиента журналируются с уровнем Warn, остальные ошибки — с уровнем Error.
func WithLogging(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		status := c.Writer.Status()
		err := c.Errors.Last()
		fields := createFields(c, startedAt)
		if err != nil {
			if status < 500 && status >= 400 {
				log.Warn("request not executed", append(fields, "error", err.Err)...)
			} else {
				log.Error("request failed via unexpected error", append(fields, "error", err.Err)...)
			}
			return
		}
		log.Info("request completed", fields...)
	}
}

// createFields создает поля для логирования на основе контекста запроса и времени начала обработки
func createFields(c *gin.Context, startedAt time.Time) logFields {
	handlerName := c.GetString(handlerNameKey)
	if handlerName == "" {
		handlerName = unknownHandlerName
	}
	return logFields{
		"handler", handlerName,
		"method", c.Request.Method,
		"uri", c.Request.URL.RequestURI(),
		"status", c.Writer.Status(),
		"response_size", c.Writer.Size(),
		"duration", time.Since(startedAt),
	}
}
