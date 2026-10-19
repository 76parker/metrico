// Package signature вычисляет подписи HTTP-тел по общему ключу сервера и агента.
package signature

import (
	"crypto/sha256"
	"encoding/hex"
)

// HeaderName задаёт HTTP-заголовок с шестнадцатеричной подписью тела запроса или ответа.
const HeaderName = "HashSHA256"

// Sum возвращает SHA-256 от последовательности байтов body и key в hex-формате.
func Sum(body []byte, key string) string {
	hash := sha256.New()
	_, _ = hash.Write(body)
	_, _ = hash.Write([]byte(key))

	return hex.EncodeToString(hash.Sum(nil))
}
