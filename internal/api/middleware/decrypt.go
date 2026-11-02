package middleware

import (
	"bytes"
	"crypto/rsa"
	"io"
	"net/http"

	"github.com/76Parker/metrico/internal/cryptomsg"
)

// DecryptRequest decrypts marked requests before they reach the router.
func DecryptRequest(next http.Handler, key *rsa.PrivateKey) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values(cryptomsg.Header)
		if len(values) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		if len(values) != 1 || values[0] != cryptomsg.Version || key == nil || r.Body == nil {
			http.Error(w, "invalid encrypted request", http.StatusBadRequest)
			return
		}
		limited := http.MaxBytesReader(w, r.Body, maxBodySize)
		body, err := io.ReadAll(limited)
		_ = limited.Close()
		if err != nil {
			http.Error(w, "invalid encrypted request", http.StatusBadRequest)
			return
		}
		plain, err := cryptomsg.Decrypt(key, body)
		if err != nil {
			http.Error(w, "invalid encrypted request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(plain))
		r.ContentLength = int64(len(plain))
		r.Header.Del(cryptomsg.Header)
		r.Header.Del("Content-Length")
		next.ServeHTTP(w, r)
	})
}
