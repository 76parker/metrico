package api_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/76Parker/metrico/internal/adapters/memstorage"
	"github.com/76Parker/metrico/internal/api"
	"github.com/76Parker/metrico/internal/api/handlers"
	"github.com/76Parker/metrico/internal/applications/health"
	"github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/pkg/logger"
)

func ExampleNewRouter_updateGauge() {
	handler := newExampleHandler()
	response := exampleRequest(handler, http.MethodPost, "/update/gauge/Alloc/42.5", "")
	fmt.Println(response.Code)

	response = exampleRequest(handler, http.MethodGet, "/value/gauge/Alloc", "")
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// 42.5
}

func ExampleNewRouter_updateCounter() {
	handler := newExampleHandler()
	for _, delta := range []string{"2", "3"} {
		response := exampleRequest(handler, http.MethodPost, "/update/counter/PollCount/"+delta, "")
		fmt.Println(response.Code)
	}
	response := exampleRequest(handler, http.MethodGet, "/value/counter/PollCount", "")
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// 200
	// 5
}

func ExampleNewRouter_getMetric() {
	handler := newExampleHandler()
	exampleRequest(handler, http.MethodPost, "/update/gauge/Alloc/42.5", "")

	response := exampleRequest(handler, http.MethodGet, "/value/gauge/Alloc", "")
	fmt.Println(response.Code)
	fmt.Println(response.Header().Get("Content-Type"))
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// text/plain; charset=utf-8
	// 42.5
}

func ExampleNewRouter_updateJSON() {
	handler := newExampleHandler()
	response := exampleRequest(handler, http.MethodPost, "/update/",
		`{"id":"Alloc","type":"gauge","value":42.5}`)
	fmt.Println(response.Code)
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// {"id":"Alloc","type":"gauge","value":42.5}
}

func ExampleNewRouter_getJSON() {
	handler := newExampleHandler()
	exampleRequest(handler, http.MethodPost, "/update/",
		`{"id":"PollCount","type":"counter","delta":3}`)

	response := exampleRequest(handler, http.MethodPost, "/value/",
		`{"id":"PollCount","type":"counter"}`)
	fmt.Println(response.Code)
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// {"id":"PollCount","type":"counter","delta":3}
}

func ExampleNewRouter_batchUpdate() {
	handler := newExampleHandler()
	response := exampleRequest(handler, http.MethodPost, "/updates/", `[
		{"id":"Alloc","type":"gauge","value":42.5},
		{"id":"PollCount","type":"counter","delta":2},
		{"id":"PollCount","type":"counter","delta":3}
	]`)
	fmt.Println(response.Code)
	fmt.Println("response bytes:", response.Body.Len())

	response = exampleRequest(handler, http.MethodPost, "/value/",
		`{"id":"PollCount","type":"counter"}`)
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// response bytes: 0
	// {"id":"PollCount","type":"counter","delta":5}
}

func ExampleNewRouter_listMetrics() {
	handler := newExampleHandler()
	exampleRequest(handler, http.MethodPost, "/updates/", `[
		{"id":"PollCount","type":"counter","delta":1},
		{"id":"Alloc","type":"gauge","value":42.5}
	]`)

	response := exampleRequest(handler, http.MethodGet, "/", "")
	fmt.Println(response.Code)
	fmt.Println(response.Header().Get("Content-Type"))
	fmt.Println(strings.Join(strings.Fields(response.Body.String()), " "))
	// Output:
	// 200
	// text/html; charset=utf-8
	// <div>Alloc: 42.5</div> <div>PollCount: 1</div>
}

func ExampleNewRouter_ping() {
	handler := newExampleHandler()
	response := exampleRequest(handler, http.MethodGet, "/ping", "")
	fmt.Println(response.Code)
	fmt.Println("response bytes:", response.Body.Len())
	// Output:
	// 200
	// response bytes: 0
}

func ExampleNewRouter_gzip() {
	handler := newExampleHandler()
	exampleRequest(handler, http.MethodPost, "/update/",
		`{"id":"Alloc","type":"gauge","value":42.5}`)

	request := httptest.NewRequest(http.MethodPost, "/value/",
		strings.NewReader(`{"id":"Alloc","type":"gauge"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		panic(err)
	}
	if err := reader.Close(); err != nil {
		panic(err)
	}
	fmt.Println(response.Code)
	fmt.Println(response.Header().Get("Content-Encoding"))
	fmt.Println(string(body))
	// Output:
	// 200
	// gzip
	// {"id":"Alloc","type":"gauge","value":42.5}
}

func ExampleNewRouter_invalidJSON() {
	handler := newExampleHandler()
	response := exampleRequest(handler, http.MethodPost, "/update/",
		`{"id":"Alloc","type":"gauge","value":42.5,"unknown":true}`)
	fmt.Println(response.Code)
	fmt.Println(response.Body.String())
	// Output:
	// 400
	// {"message":"invalid input json","code":"invalid_json","status":400}
}

// newExampleHandler собирает настоящий HTTP-роутер с хранилищем в памяти.
// NewRouter загружает templates/* из корня проекта; каталог восстанавливается
// сразу после создания роутера, до выполнения запросов.
func newExampleHandler() http.Handler {
	workingDirectory, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if err := os.Chdir("../.."); err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Chdir(workingDirectory); err != nil {
			panic(err)
		}
	}()
	defer gin.SetMode(gin.Mode())

	log := logger.NewMockLogger()
	application := metrics.NewApplication(metrics.NewService(memstorage.NewMemStorage()))
	availability := health.NewApplication(examplePinger{})
	return api.NewRouter(
		handlers.NewMetricsHandler(application),
		handlers.NewHealthHandler(log, availability),
		config.HTTP{},
		log,
	).Handler
}

// examplePinger заменяет только внешнюю зависимость проверки доступности.
type examplePinger struct{}

func (examplePinger) Ping(context.Context) error { return nil }

func exampleRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
