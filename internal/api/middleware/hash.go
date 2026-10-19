package middleware

import (
	"bytes"
	"crypto/subtle"
	"io"
	"net/http"

	"github.com/76Parker/metrico/internal/signature"
)

const maxBodySize = 1 << 20

type responseBuffer struct {
	body   bytes.Buffer
	header http.Header
	status int
}

func newResponseBuffer() *responseBuffer {
	return &responseBuffer{
		header: http.Header{},
	}
}

func (b *responseBuffer) Header() http.Header {
	return b.header
}

func (b *responseBuffer) WriteHeader(status int) {
	if b.status != 0 {
		return
	}
	b.status = status
}

func (b *responseBuffer) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}

	return b.body.Write(data)
}

func (b *responseBuffer) WriteString(data string) (int, error) {
	return b.Write([]byte(data))
}

// VerifyAndSign проверяет переданную подпись HashSHA256 и подписывает тело ответа.
// Запросы без подписи допускаются; неверная подпись даёт HTTP 400.
// При пустом ключе возвращает next без дополнительной обработки.
func VerifyAndSign(next http.Handler, key string) http.Handler {
	if key == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if actualSignature := r.Header.Get(signature.HeaderName); actualSignature != "" {
			body, err := readAndRestoreBody(w, r)
			if err != nil || !isValidSignature(actualSignature, body, key) {
				writeSignedResponse(w, http.StatusBadRequest, nil, key, r.Method)
				return
			}
		}

		buffer := newResponseBuffer()
		next.ServeHTTP(buffer, r)

		writeSignedResponse(w, buffer.status, buffer, key, r.Method)
	})
}

func readAndRestoreBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	limitedBody := http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, err
	}
	if err := limitedBody.Close(); err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	return body, nil
}

func isValidSignature(actual string, body []byte, key string) bool {
	expected := signature.Sum(body, key)

	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func writeSignedResponse(
	w http.ResponseWriter,
	status int,
	buffer *responseBuffer,
	key string,
	requestMethod string,
) {
	if status == 0 {
		status = http.StatusOK
	}
	body := []byte(nil)
	if buffer != nil {
		copyHeaders(w.Header(), buffer.header)
		body = buffer.body.Bytes()
	}
	if !responseBodyAllowed(requestMethod, status) {
		body = nil
	}
	w.Header().Set(signature.HeaderName, signature.Sum(body, key))
	w.WriteHeader(status)
	if len(body) == 0 {
		return
	}
	_, _ = w.Write(body)
}

func responseBodyAllowed(requestMethod string, status int) bool {
	if requestMethod == http.MethodHead {
		return false
	}

	switch status {
	case http.StatusNoContent, http.StatusNotModified:
		return false
	}

	return status >= http.StatusOK
}

func copyHeaders(destination http.Header, source http.Header) {
	for name, values := range source {
		destination.Del(name)
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}
