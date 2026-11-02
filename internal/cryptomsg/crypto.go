// Package cryptomsg implements the encrypted agent request envelope.
package cryptomsg

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
)

const Header = "X-Metrico-Encryption"
const Version = "1"

func readPEM(path string) (*pem.Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid PEM")
	}
	return block, nil
}
func LoadPublic(path string) (*rsa.PublicKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}
	var key *rsa.PublicKey
	switch block.Type {
	case "PUBLIC KEY":
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		var ok bool
		key, ok = parsed.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("not an RSA public key")
		}
	case "RSA PUBLIC KEY":
		key, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported public key PEM")
	}
	if key.N == nil || key.E < 2 || key.Size() < 2*sha256.Size+2+32 || key.Size() > 65535 {
		return nil, errors.New("invalid or unsupported RSA public key")
	}
	return key, nil
}
func LoadPrivate(path string) (*rsa.PrivateKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			var ok bool
			key, ok = parsed.(*rsa.PrivateKey)
			if !ok {
				return nil, errors.New("not an RSA private key")
			}
		}
	default:
		return nil, errors.New("unsupported private key PEM")
	}
	if err != nil {
		return nil, err
	}
	if err = key.Validate(); err != nil {
		return nil, err
	}
	if key.Size() < 2*sha256.Size+2+32 || key.Size() > 65535 {
		return nil, errors.New("unsupported RSA key size")
	}
	return key, nil
}

// Encrypt produces version byte, uint16 big-endian RSA ciphertext length, RSA
// ciphertext, 12-byte GCM nonce and authenticated ciphertext (including tag).
func Encrypt(key *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return nil, err
	}
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, secret, nil)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 3, 3+len(encryptedKey)+len(nonce)+len(plaintext)+gcm.Overhead())
	out[0] = 1
	binary.BigEndian.PutUint16(out[1:3], uint16(len(encryptedKey)))
	out = append(out, encryptedKey...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}
func Decrypt(key *rsa.PrivateKey, envelope []byte) ([]byte, error) {
	if len(envelope) < 3 || envelope[0] != 1 {
		return nil, errors.New("invalid envelope")
	}
	n := int(binary.BigEndian.Uint16(envelope[1:3]))
	if n != key.Size() || len(envelope) < 3+n+12+16 {
		return nil, errors.New("invalid envelope length")
	}
	secret, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, envelope[3:3+n], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt key: %w", err)
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	offset := 3 + n
	return gcm.Open(nil, envelope[offset:offset+gcm.NonceSize()], envelope[offset+gcm.NonceSize():], nil)
}
