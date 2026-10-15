package middleware

import "github.com/gin-gonic/gin"

const handlerNameKey = "handler_name"

const unknownHandlerName = "unknown"

// HandlerName сохраняет имя операции в контексте Gin для журналирования запроса.
func HandlerName(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(handlerNameKey, op)
		c.Next()
	}
}
