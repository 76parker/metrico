// Package middleware содержит обработку ошибок, сжатие, подписи и журналирование HTTP-запросов.
package middleware

import (
	"compress/gzip"
	"io"
	"mime"
	"sync"

	"github.com/gin-gonic/gin"
)

type gzipCompressor struct {
	gin.ResponseWriter
	writer *gzip.Writer
	pool   *sync.Pool
}

func (g *gzipCompressor) Write(p []byte) (int, error) {
	if g.writer == nil {
		if !shouldCompress(g.Header().Get("Content-Type")) {
			return g.ResponseWriter.Write(p)
		}

		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Del("Content-Length")

		if pooled := g.pool.Get(); pooled != nil {
			g.writer = pooled.(*gzip.Writer)
			g.writer.Reset(g.ResponseWriter)
		} else {
			writer, err := gzip.NewWriterLevel(g.ResponseWriter, gzip.BestSpeed)
			if err != nil {
				return 0, err
			}
			g.writer = writer
		}
	}

	return g.writer.Write(p)
}

func (g *gzipCompressor) WriteString(s string) (int, error) {
	return g.Write([]byte(s))
}

func shouldCompress(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return mediaType == "text/html" || mediaType == "application/json"
}

// Compress сжимает JSON- и HTML-ответы, если Accept-Encoding в точности равен gzip.
// Каждый экземпляр middleware владеет пулом gzip.Writer для повторного использования
// компрессоров без совместного доступа к одному компрессору из разных запросов.
func Compress() gin.HandlerFunc {
	// Each middleware instance owns its pool; a request exclusively owns a borrowed writer.
	pool := &sync.Pool{}
	return gin.HandlerFunc(func(c *gin.Context) {
		if c.GetHeader("Accept-Encoding") != "gzip" {
			c.Next()
			return
		}

		writer := &gzipCompressor{
			ResponseWriter: c.Writer,
			pool:           pool,
		}
		c.Writer = writer
		defer func() {
			if writer.writer != nil {
				_ = writer.writer.Close()
				// The pool must not retain the HTTP response or its connection between requests.
				writer.writer.Reset(io.Discard)
				pool.Put(writer.writer)
				writer.writer = nil
			}
		}()
		c.Next()
	})
}
