// Package domain содержит основные типы и интерфейсы системы.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// User представляет пользователя системы.
type User struct {
	ID           uuid.UUID
	Login        string
	PasswordHash []byte
	PasswordSalt []byte
	// KDF — параметры Argon2id, с которыми из пароля выводится мастер-ключ.
	KDF KDFParams
	// TokenVersion — версия токенов пользователя; токены с другой версией недействительны.
	TokenVersion int
	// PasswordExpiresAt задан для временного пароля, выданного администратором:
	// до этого времени им можно войти, после входа пароль нужно сменить.
	PasswordExpiresAt *time.Time
	CreatedAt         time.Time
}

// Secret представляет зашифрованный секрет пользователя.
type Secret struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	BlindIndex string
	Data       []byte
	UpdatedAt  time.Time
	CreatedAt  time.Time
}

// UserInfo — сведения о пользователе для администратора.
type UserInfo struct {
	Login       string
	CreatedAt   time.Time
	SecretCount int
	// PasswordExpiresAt задан, если пароль временный.
	PasswordExpiresAt *time.Time
	KDF               KDFParams
}

// UserPage — страница списка пользователей.
type UserPage struct {
	Users []*UserInfo
	// NextPageToken — курсор следующей страницы; пустой, если страница последняя.
	NextPageToken string
}

// Registration — данные регистрации пользователя.
type Registration struct {
	Login   string
	AuthKey []byte
	Salt    []byte
	KDF     KDFParams
	// Temporary — пароль временный: ограничен по времени и должен быть сменён при входе.
	Temporary bool
}

// LoginResult — результат входа.
type LoginResult struct {
	Token string
	// PasswordChangeRequired — пароль временный; Token разрешает только смену пароля.
	PasswordChangeRequired bool
}

// PasswordChange — данные для смены пароля, которые клиент выводит из старого и нового пароля.
type PasswordChange struct {
	// OldAuthKey — ключ аутентификации от текущего пароля.
	OldAuthKey []byte
	// NewSalt и NewKDF — соль и параметры Argon2id нового пароля.
	NewSalt []byte
	NewKDF  KDFParams
	// NewAuthKey — ключ аутентификации от нового пароля.
	NewAuthKey []byte
}

// PasswordHashChange — то, что сохраняется в БД при смене пароля.
type PasswordHashChange struct {
	// OldHash — хеш ключа аутентификации от текущего пароля; смена выполняется,
	// только если он совпадает с сохранённым.
	OldHash []byte
	NewHash []byte
	NewSalt []byte
	NewKDF  KDFParams
}

// ReencryptedSecret — секрет, перешифрованный клиентом ключом от нового пароля.
type ReencryptedSecret struct {
	OldBlindIndex string
	NewBlindIndex string
	Data          []byte
	// ExpectedUpdatedAt — updated_at секрета на момент чтения клиентом.
	ExpectedUpdatedAt time.Time
}

// SecretIterator возвращает следующий перешифрованный секрет или (nil, nil) в конце.
// Позволяет обрабатывать секреты по мере получения, не держа их все в памяти.
type SecretIterator func() (*ReencryptedSecret, error)

// SecretPage — страница списка секретов.
type SecretPage struct {
	Secrets []*Secret
	// NextPageToken — курсор следующей страницы; пустой, если страница последняя.
	NextPageToken string
}

// SecretType определяет тип хранимого секрета.
type SecretType string

const (
	SecretTypeCredentials SecretType = "credentials" // пары логин/пароль
	SecretTypeText        SecretType = "text"        // произвольный текст
	SecretTypeBinary      SecretType = "binary"      // произвольные бинарные данные
	SecretTypeCard        SecretType = "card"        // данные банковской карты
)

// ErrUnknownSecretType возвращается при попытке использовать неизвестный тип секрета.
var ErrUnknownSecretType = errors.New("unknown secret type")

// ParseSecretType валидирует строку и возвращает SecretType.
// Возвращает ErrUnknownSecretType, если значение не соответствует ни одному типу.
func ParseSecretType(s string) (SecretType, error) {
	switch SecretType(s) {
	case SecretTypeCredentials, SecretTypeText, SecretTypeBinary, SecretTypeCard:
		return SecretType(s), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownSecretType, s)
	}
}

// CredentialsSecret представляет секрет с парами логин/пароль.
type CredentialsSecret struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	// URIs — сайты и приложения, для которых этот логин: «github.com»,
	// «https://github.com/login», «androidapp://com.github.android» (AndroidAppURIPrefix).
	// По ним клиенты подбирают логин при автозаполнении. Поле появилось позже остальных:
	// у старых секретов его нет, а клиент, который о нём не знает, при изменении секрета
	// перезапишет данные без него.
	URIs []string `json:"uris,omitempty"`
}

// TextSecret представляет секрет с произвольными текстовыми данными.
type TextSecret struct {
	Text string `json:"text"`
}

// BinarySecret представляет секрет с произвольными бинарными данными.
type BinarySecret struct {
	Data []byte `json:"data"`
}

// CardSecret представляет секрет с данными банковских карт.
type CardSecret struct {
	Number string `json:"number"`
	Holder string `json:"holder"`
	Expiry string `json:"expiry"`
	CVV    string `json:"cvv"`
}

// SecretPayload представляет секрет для шифрования.
type SecretPayload struct {
	Name     string          `json:"name"`
	Type     SecretType      `json:"type"`
	Data     json.RawMessage `json:"data"`
	Metadata string          `json:"metadata,omitempty"`
	// Group — группа (папка) секрета для порядка в списке; пусто — без группы.
	// Хранится внутри зашифрованных данных, поэтому сервер о группах не знает.
	// Название приводится к виду NormalizeGroup. Поле появилось позже остальных:
	// клиент, который о нём не знает, при изменении секрета перезапишет его без группы.
	Group string `json:"group,omitempty"`
}

// SecretInfo представляет секрет для отображения пользователю.
type SecretInfo struct {
	SecretPayload
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Credentials содержит учётные данные пользователя.
type Credentials struct {
	Login string
	// AuthKey — ключ аутентификации HKDF(masterKey, "auth"), отправляемый на сервер.
	// Не позволяет восстановить masterKey и ключи шифрования секретов.
	AuthKey []byte
}
