package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func BenchmarkCompress(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, size := range []int{1, 50, 500} {
		b.Run(fmt.Sprintf("metrics=%d", size), func(b *testing.B) {
			batch := make([]struct {
				ID    string  `json:"id"`
				Type  string  `json:"type"`
				Value float64 `json:"value"`
			}, size)
			for i := range batch {
				batch[i].ID = fmt.Sprintf("metric_%04d", i)
				batch[i].Type = "gauge"
				batch[i].Value = float64(i)
			}
			body, err := json.Marshal(batch)
			if err != nil {
				b.Fatal(err)
			}
			router := gin.New()
			router.Use(Compress())
			router.GET("/", func(ctx *gin.Context) {
				ctx.Data(http.StatusOK, "application/json", body)
			})
			request, err := http.NewRequest(http.MethodGet, "/", nil)
			if err != nil {
				b.Fatal(err)
			}
			request.Header.Set("Accept-Encoding", "gzip")
			response := &benchmarkResponse{header: make(http.Header)}
			b.ReportAllocs()
			for b.Loop() {
				clear(response.header)
				response.body.Reset()
				router.ServeHTTP(response, request)
			}
		})
	}
}

type benchmarkResponse struct {
	header http.Header
	body   bytes.Buffer
}

func (r *benchmarkResponse) Header() http.Header            { return r.header }
func (r *benchmarkResponse) WriteHeader(int)                {}
func (r *benchmarkResponse) Write(data []byte) (int, error) { return r.body.Write(data) }
