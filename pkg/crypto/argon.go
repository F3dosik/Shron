package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/F3dosik/Shron/internal/domain"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

// keyLen — длина мастер-ключа в байтах.
const keyLen = 32

// Пустая соль для явной передачи в hkdf.
var noSalt []byte

const (
	// InfoEncryption используется для деривации ключа шифрования AES-256-GCM.
	InfoEncryption = "encryption"
	// InfoBlindIndex используется для деривации ключа HMAC-SHA256 blind index.
	InfoBlindIndex = "blind-index"
	// InfoAuth используется для деривации ключа аутентификации, который отправляется на сервер.
	// Ключ независим от ключей шифрования и blind index: знание authKey не позволяет их восстановить.
	InfoAuth = "auth"
	// InfoOfflineCache используется для деривации ключа HMAC-SHA256, которым подписана
	// офлайн-копия хранилища на устройстве (internal/client/offline).
	InfoOfflineCache = "offline-cache"
)

// DeriveKey возвращает ключ из пароля и соли используя Argon2id с параметрами params.
// Параметры должны быть проверены вызывающим (domain.KDFParams.Validate).
func DeriveKey(password string, salt []byte, params domain.KDFParams) []byte {
	return argon2.IDKey([]byte(password), salt, params.Time, params.MemoryKiB, params.Threads, keyLen)
}

// GenerateSalt генерирует случайную соль длиной 16 байт.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return salt, nil
}

// HashAuthKey возвращает SHA-256 от authKey — значение, которое сервер хранит в БД.
// Медленный хеш не нужен: authKey — 32 случайных байта, полученных через Argon2id на клиенте,
// поэтому перебор по хешу невозможен, а утечка хеша не даёт пройти аутентификацию.
func HashAuthKey(authKey []byte) []byte {
	sum := sha256.Sum256(authKey)
	return sum[:]
}

// HKDF выводит ключ длиной 32 байта из masterKey используя HKDF-SHA256.
// info задаёт контекст деривации и гарантирует независимость ключей.
func HKDF(materKey []byte, info string) ([]byte, error) {
	reader := hkdf.New(sha256.New, materKey, noSalt, []byte(info))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("hkdf exapnd: %w", err)
	}
	return key, nil
}
