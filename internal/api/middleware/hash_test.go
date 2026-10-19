package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/76Parker/metrico/internal/signature"
	"github.com/gin-gonic/gin"
)

func TestVerifyAndSign(t *testing.T) {
	const key = "test-key"

	t.Run("valid request is passed through and response is signed", func(t *testing.T) {
		requestBody := []byte(`[{"id":"Alloc"}]`)
		responseBody := []byte(`{"ok":true}`)
		handlerCalled := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if string(body) != string(requestBody) {
				t.Fatalf("handler received body %q, want %q", body, requestBody)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(responseBody)
		})

		request := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(requestBody))
		request.Header.Set(signature.HeaderName, signature.Sum(requestBody, key))
		response := httptest.NewRecorder()

		VerifyAndSign(handler, key).ServeHTTP(response, request)

		if !handlerCalled {
			t.Fatal("handler was not called")
		}
		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
		}
		if got := response.Header().Get(signature.HeaderName); got != signature.Sum(responseBody, key) {
			t.Fatalf("response hash = %q, want %q", got, signature.Sum(responseBody, key))
		}
	})

	t.Run("invalid request is rejected before handler", func(t *testing.T) {
		handlerCalled := false
		handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			handlerCalled = true
		})
		request := httptest.NewRequest(http.MethodPost, "/updates", nil)
		request.Header.Set(signature.HeaderName, "invalid")
		response := httptest.NewRecorder()

		VerifyAndSign(handler, key).ServeHTTP(response, request)

		if handlerCalled {
			t.Fatal("handler was called for an invalid request")
		}
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
		}
		if got := response.Header().Get(signature.HeaderName); got != signature.Sum(nil, key) {
			t.Fatalf("response hash = %q, want %q", got, signature.Sum(nil, key))
		}
	})

	t.Run("unsigned request is passed through and response is signed", func(t *testing.T) {
		requestBody := []byte(`{"id":"Alloc"}`)
		responseBody := []byte(`{"value":1}`)
		handlerCalled := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if string(body) != string(requestBody) {
				t.Fatalf("handler received body %q, want %q", body, requestBody)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(responseBody)
		})
		request := httptest.NewRequest(http.MethodPost, "/value/", bytes.NewReader(requestBody))
		response := httptest.NewRecorder()

		VerifyAndSign(handler, key).ServeHTTP(response, request)

		if !handlerCalled {
			t.Fatal("handler was not called")
		}
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if got := response.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q, want %q", got, "application/json")
		}
		if got := response.Header().Get(signature.HeaderName); got != signature.Sum(responseBody, key) {
			t.Fatalf("response hash = %q, want %q", got, signature.Sum(responseBody, key))
		}
	})

	t.Run("empty key leaves requests and responses unchanged", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		request := httptest.NewRequest(http.MethodGet, "/ping", nil)
		response := httptest.NewRecorder()

		VerifyAndSign(handler, "").ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
		}
		if got := response.Header().Get(signature.HeaderName); got != "" {
			t.Fatalf("response hash = %q, want empty", got)
		}
	})

	for _, tt := range []struct {
		name   string
		method string
		status int
	}{
		{name: "head response", method: http.MethodHead, status: http.StatusOK},
		{name: "no content response", method: http.MethodGet, status: http.StatusNoContent},
		{name: "not modified response", method: http.MethodGet, status: http.StatusNotModified},
	} {
		t.Run(tt.name+" signs empty transmitted body", func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("not transmitted"))
			})
			request := httptest.NewRequest(tt.method, "/metrics", nil)
			request.Header.Set(signature.HeaderName, signature.Sum(nil, key))
			response := httptest.NewRecorder()

			VerifyAndSign(handler, key).ServeHTTP(response, request)

			if got := response.Header().Get(signature.HeaderName); got != signature.Sum(nil, key) {
				t.Fatalf("response hash = %q, want %q", got, signature.Sum(nil, key))
			}
			if got := response.Body.Len(); got != 0 {
				t.Fatalf("response body size = %d, want 0", got)
			}
		})
	}
}

func TestVerifyAndSign_RequestTooLarge(t *testing.T) {
	const key = "test-key"
	requestBody := bytes.Repeat([]byte("a"), 1<<20+1)
	handlerCalled := false
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		handlerCalled = true
	})
	request := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(requestBody))
	request.Header.Set(signature.HeaderName, signature.Sum(requestBody, key))
	response := httptest.NewRecorder()

	VerifyAndSign(handler, key).ServeHTTP(response, request)

	if handlerCalled {
		t.Fatal("handler was called for an oversized request")
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if got := response.Header().Get(signature.HeaderName); got != signature.Sum(nil, key) {
		t.Fatalf("response hash = %q, want %q", got, signature.Sum(nil, key))
	}
}

func TestVerifyAndSign_CompressedResponse(t *testing.T) {
	const key = "test-key"
	responseBody := []byte(`{"ok":true}`)
	router := gin.New()
	router.Use(Compress())
	router.GET("/metrics", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", responseBody)
	})
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set(signature.HeaderName, signature.Sum(nil, key))
	response := httptest.NewRecorder()

	VerifyAndSign(router, key).ServeHTTP(response, request)

	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q, want %q", got, "gzip")
	}
	if got := response.Header().Get(signature.HeaderName); got != signature.Sum(response.Body.Bytes(), key) {
		t.Fatalf("response hash = %q, want %q", got, signature.Sum(response.Body.Bytes(), key))
	}
	reader, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("open gzip response: %v", err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip response: %v", err)
	}
	if string(body) != string(responseBody) {
		t.Fatalf("response body = %q, want %q", body, responseBody)
	}
}
