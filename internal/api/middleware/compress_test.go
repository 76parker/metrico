package middleware

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCompressOnlySupportedContentTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		contentType string
		body        string
		compressed  bool
	}{
		{
			name:        "valid/compresses_json_with_charset",
			contentType: "application/json; charset=utf-8",
			body:        `{"status":"ok"}`,
			compressed:  true,
		},
		{
			name:        "valid/compresses_html_with_charset",
			contentType: "text/html; charset=utf-8",
			body:        "<h1>Hello</h1>",
			compressed:  true,
		},
		{
			name:        "valid/does_not_compress_plain_text",
			contentType: "text/plain; charset=utf-8",
			body:        "Hello",
			compressed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(Compress())
			router.GET("/", func(c *gin.Context) {
				c.Header("Content-Type", tt.contentType)
				c.String(http.StatusOK, tt.body)
			})

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Accept-Encoding", "gzip")
			router.ServeHTTP(response, request)

			if !tt.compressed {
				require.Empty(t, response.Header().Get("Content-Encoding"))
				require.Equal(t, tt.body, response.Body.String())
				return
			}

			require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
			reader, err := gzip.NewReader(response.Body)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, reader.Close()) })

			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tt.body, strings.TrimSpace(string(body)))
		})
	}
}

func TestCompress_SequentialAndConcurrentResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Compress())
	router.GET("/:id", func(ctx *gin.Context) {
		ctx.Header("Content-Length", "999999")
		ctx.Header("X-Test", "preserved")
		ctx.Header("Content-Type", "application/json")
		_, err := ctx.Writer.WriteString(`{"id":"` + ctx.Param("id") + `","value":"`)
		if err != nil {
			_ = ctx.Error(err)
			return
		}
		_, err = ctx.Writer.Write([]byte(strings.Repeat(ctx.Param("id"), 32) + `"}`))
		if err != nil {
			_ = ctx.Error(err)
		}
	})
	check := func(id string) error {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/"+id, nil)
		request.Header.Set("Accept-Encoding", "gzip")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Encoding") != "gzip" || response.Header().Get("Content-Length") != "" || response.Header().Get("X-Test") != "preserved" {
			return fmt.Errorf("invalid response status/headers: %d %v", response.Code, response.Header())
		}
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(reader)
		closeError := reader.Close()
		if err != nil || closeError != nil {
			return errors.Join(err, closeError)
		}
		expected := `{"id":"` + id + `","value":"` + strings.Repeat(id, 32) + `"}`
		if string(body) != expected {
			return fmt.Errorf("response = %q, want %q", body, expected)
		}
		return nil
	}
	for index := range 20 {
		require.NoError(t, check(fmt.Sprintf("sequential-%d", index)))
	}
	var workers sync.WaitGroup
	for client := range 8 {
		workers.Go(func() {
			for index := range 20 {
				if err := check(fmt.Sprintf("client-%d-response-%d", client, index)); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	workers.Wait()
}

func TestCompress_EmptyResponsesAndEncodingNegotiation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name       string
		encoding   string
		body       string
		compressed bool
	}{
		{"no encoding", "", `{"ok":true}`, false},
		{"exact gzip", "gzip", `{"ok":true}`, true},
		{"encoding list preserves existing behavior", "gzip, deflate", `{"ok":true}`, false},
		{"empty response", "gzip", "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(Compress())
			router.GET("/", func(ctx *gin.Context) {
				ctx.Header("Content-Type", "application/json")
				if testCase.body != "" {
					_, err := ctx.Writer.Write([]byte(testCase.body))
					require.NoError(t, err)
				}
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Accept-Encoding", testCase.encoding)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, testCase.compressed, response.Header().Get("Content-Encoding") == "gzip")
			if !testCase.compressed {
				require.Equal(t, testCase.body, response.Body.String())
			}
		})
	}
}

type failingResponse struct {
	header           http.Header
	err              error
	successfulWrites int
}

func (r *failingResponse) Header() http.Header { return r.header }
func (*failingResponse) WriteHeader(int)       {}
func (r *failingResponse) Write(data []byte) (int, error) {
	if r.successfulWrites > 0 {
		r.successfulWrites--
		return len(data), nil
	}
	return 0, r.err
}

func TestCompress_RecoversAfterWriteFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, failure := range []string{"write", "close"} {
		t.Run(failure, func(t *testing.T) {
			writeError := errors.New("response write failed")
			var observedError error
			router := gin.New()
			router.Use(Compress())
			router.GET("/", func(ctx *gin.Context) {
				ctx.Header("Content-Type", "application/json")
				_, observedError = ctx.Writer.Write([]byte(`{"ok":true}`))
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Accept-Encoding", "gzip")
			failing := &failingResponse{header: make(http.Header), err: writeError}
			if failure == "close" {
				failing.successfulWrites = 1
			}
			router.ServeHTTP(failing, request)
			if failure == "write" {
				require.ErrorIs(t, observedError, writeError)
			} else {
				require.NoError(t, observedError)
				require.Zero(t, failing.successfulWrites, "header write must succeed before close fails")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.NoError(t, observedError)
			reader, err := gzip.NewReader(response.Body)
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			require.JSONEq(t, `{"ok":true}`, string(body))
		})
	}
}

func TestCompress_ExplicitEmptyWriteProducesValidStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Compress())
	router.GET("/", func(ctx *gin.Context) {
		ctx.Header("Content-Type", "application/json")
		written, err := ctx.Writer.Write(nil)
		require.NoError(t, err)
		require.Zero(t, written)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
	reader, err := gzip.NewReader(response.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Empty(t, body)
	require.NoError(t, reader.Close())
}
