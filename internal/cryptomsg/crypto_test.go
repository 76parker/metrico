package cryptomsg

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvelope(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	plain := bytes.Repeat([]byte("metric"), 10000)
	encrypted, err := Encrypt(&key.PublicKey, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, plain) {
		t.Fatal("plaintext visible")
	}
	decoded, err := Decrypt(key, encrypted)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatalf("round trip: %v", err)
	}
	if _, err = Decrypt(other, encrypted); err == nil {
		t.Fatal("accepted wrong key")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err = Decrypt(key, encrypted); err == nil {
		t.Fatal("accepted corrupted ciphertext")
	}
	for _, bad := range [][]byte{nil, {1, 0, 1}, {2, 0, 0}} {
		if _, err := Decrypt(key, bad); err == nil {
			t.Fatal("accepted invalid envelope")
		}
	}
}
func TestLoadKeys(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkix, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		typ    string
		data   []byte
		public bool
	}{
		{"PUBLIC KEY", pkix, true}, {"RSA PUBLIC KEY", x509.MarshalPKCS1PublicKey(&key.PublicKey), true},
		{"PRIVATE KEY", pkcs8, false}, {"RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), false},
	}
	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key.pem")
			if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: tc.typ, Bytes: tc.data}), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.public {
				_, err = LoadPublic(path)
			} else {
				_, err = LoadPrivate(path)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := LoadPublic(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted missing key")
	}
}
