package middleware

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/76Parker/metrico/internal/cryptomsg"
	"github.com/76Parker/metrico/internal/signature"
)

func TestEncryptedRequest(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	plain := bytes.Repeat([]byte("metrics"), 1000)
	encrypted, err := cryptomsg.Encrypt(&key.PublicKey, plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, signed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsigned", true: "signed"}[signed], func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if !bytes.Equal(body, plain) {
					t.Error("body mismatch")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(encrypted))
			req.Header.Set(cryptomsg.Header, cryptomsg.Version)
			hash := ""
			if signed {
				hash = "secret"
				req.Header.Set(signature.HeaderName, signature.Sum(encrypted, hash))
			}
			w := httptest.NewRecorder()
			VerifyAndSign(DecryptRequest(handler, key), hash).ServeHTTP(w, req)
			if w.Code != http.StatusNoContent {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
	for _, tc := range []struct {
		name string
		key  *rsa.PrivateKey
		body []byte
	}{{"missing key", nil, encrypted}, {"invalid", key, []byte("bad")}, {"too large", key, make([]byte, maxBodySize+1)}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tc.body))
			req.Header.Set(cryptomsg.Header, cryptomsg.Version)
			w := httptest.NewRecorder()
			DecryptRequest(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler called") }), tc.key).ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
}
