// Package offline — копия хранилища на устройстве для работы без сервера.
//
// Копия хранит записи в том виде, в каком их отдаёт сервер: blind index и данные,
// зашифрованные ключом из мастер-ключа. Поэтому на устройстве секреты защищены так же,
// как на сервере, и без мастер-пароля (или мастер-ключа) их не прочитать. Чтобы без
// сервера вывести мастер-ключ из пароля, копия хранит соль и параметры Argon2id учётки.
//
// Весь файл подписан HMAC-SHA256 ключом HKDF(masterKey, "offline-cache"). Подпись
// служит двум целям: проверяет пароль без сервера (неверный пароль даёт другой ключ)
// и не даёт незаметно подменить соль, параметры или набор записей в файле.
//
// Кроме копии записей сервера, файл хранит записи «только на этом устройстве»
// (Snapshot.Local): они зашифрованы так же, но на сервер не отправляются. Поэтому файл
// с такими записями нельзя просто удалить: если его подпись не сходится (пароль сменили
// на другом устройстве), он откладывается (Archive) до ввода прежнего пароля.
package offline

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/F3dosik/Shron/internal/domain"
	"github.com/F3dosik/Shron/pkg/crypto"
)

// FileName — имя файла копии; лежит рядом с файлом сессии.
const FileName = "offline_vault.json"

// StaleSuffix — суффикс отложенной копии (Archive): записи только на этом устройстве,
// зашифрованные прежним паролем.
const StaleSuffix = ".old"

// version — версия формата файла.
const version = 1

// ErrWrongKey — подпись копии не сошлась: неверный пароль (или ключ) либо файл изменён.
var ErrWrongKey = errors.New("offline: wrong key or corrupted cache")

// Record — запись хранилища в зашифрованном виде, как её вернул сервер.
type Record struct {
	BlindIndex string    `json:"blindIndex"`
	Data       []byte    `json:"data"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Snapshot — содержимое копии.
type Snapshot struct {
	// Server — адрес сервера, Login — учётка: копия годится только для них.
	Server string `json:"server"`
	Login  string `json:"login"`
	// Salt и KDF — соль и параметры Argon2id учётки для вывода мастер-ключа из пароля.
	// Пустая соль — копия сохранена после разблокировки ключом (биометрией) и соль
	// ещё не известна: открыть её можно только мастер-ключом.
	Salt []byte           `json:"salt,omitempty"`
	KDF  domain.KDFParams `json:"kdf"`
	// SyncedAt — когда записи последний раз получены с сервера.
	SyncedAt time.Time `json:"syncedAt"`
	// Records — записи, как они были на сервере при последнем обращении к нему.
	Records []Record `json:"records"`
	// Pending — изменения, сделанные без сервера и ещё не отправленные: по одному на
	// запись, в порядке первого изменения. Список показывает записи с их учётом.
	Pending []PendingOp `json:"pending,omitempty"`
	// Local — записи только на этом устройстве: на сервер не отправляются, в Records
	// не попадают. Blind index не пересекается с записями сервера, которые видит клиент.
	Local []Record `json:"local,omitempty"`
}

// Empty сообщает, что в копии нет ничего, кроме сведений об учётке: записи с сервера
// ни разу не получены, изменений и записей только на устройстве нет.
func (s *Snapshot) Empty() bool {
	return s.SyncedAt.IsZero() && len(s.Pending) == 0 && len(s.Local) == 0
}

// OpKind — вид изменения без сервера.
type OpKind string

// Виды изменений.
const (
	OpCreate OpKind = "create"
	OpUpdate OpKind = "update"
	OpDelete OpKind = "delete"
)

// PendingOp — изменение записи, сделанное без сервера.
type PendingOp struct {
	Kind       OpKind `json:"kind"`
	BlindIndex string `json:"blindIndex"`
	// Data — новые зашифрованные данные (create, update); для delete — данные удалённой
	// записи, чтобы показать её в конфликте. На сервер для delete не отправляются.
	Data []byte `json:"data"`
	// BaseHash — SHA-256 шифротекста записи на сервере, с которой начато изменение
	// (update, delete). Шифротекст меняется при каждом изменении (случайный nonce),
	// поэтому другой хеш на сервере значит, что запись изменили на другом устройстве.
	BaseHash []byte `json:"baseHash,omitempty"`
	// EditedAt — когда изменение сделано на устройстве.
	EditedAt time.Time `json:"editedAt"`
	// Conflict — изменение не отправлено из-за конфликта; ждёт решения пользователя.
	Conflict *Conflict `json:"conflict,omitempty"`
}

// ConflictKind — причина конфликта.
type ConflictKind string

// Причины конфликта.
const (
	// ConflictChanged — запись изменили и на устройстве, и на сервере.
	ConflictChanged ConflictKind = "changed"
	// ConflictDeletedOnServer — запись изменили на устройстве, а на сервере удалили.
	ConflictDeletedOnServer ConflictKind = "deleted_on_server"
	// ConflictExists — запись создали на устройстве, а на сервере уже есть такая.
	ConflictExists ConflictKind = "exists"
	// ConflictChangedOnServer — запись удалили на устройстве, а на сервере изменили.
	ConflictChangedOnServer ConflictKind = "changed_on_server"
	// ConflictRejected — сервер отклонил изменение (например, превышен лимит).
	ConflictRejected ConflictKind = "rejected"
)

// Conflict — сведения о конфликте.
type Conflict struct {
	Kind ConflictKind `json:"kind"`
	// ServerData и ServerUpdatedAt — версия записи на сервере; пустые, если её там нет.
	ServerData      []byte    `json:"serverData,omitempty"`
	ServerUpdatedAt time.Time `json:"serverUpdatedAt,omitempty"`
	// Message — текст ошибки сервера для ConflictRejected.
	Message string `json:"message,omitempty"`
}

// file — формат файла: подписанное тело и подпись.
type file struct {
	Version int             `json:"version"`
	Body    json.RawMessage `json:"body"`
	MAC     []byte          `json:"mac"`
}

// Sealed — прочитанная, но ещё не проверенная копия. Поля Snapshot доступны для вывода
// ключа, но доверять им можно только после Open.
type Sealed struct {
	snapshot Snapshot
	body     []byte
	mac      []byte
}

// Unverified возвращает содержимое без проверки подписи — чтобы взять соль и параметры
// Argon2id для вывода ключа. Записям и полям доверять нельзя, пока Open не вернул nil.
func (s *Sealed) Unverified() *Snapshot {
	return &s.snapshot
}

// Open проверяет подпись ключом из masterKey и возвращает содержимое.
func (s *Sealed) Open(masterKey []byte) (*Snapshot, error) {
	mac, err := sign(masterKey, s.body)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(mac, s.mac) {
		return nil, ErrWrongKey
	}
	snapshot := s.snapshot
	return &snapshot, nil
}

// Load читает копию из path. Отсутствие файла — os.ErrNotExist.
func Load(path string) (*Sealed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("offline: %w", err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("offline: файл %s повреждён: %w", path, err)
	}
	if f.Version != version {
		return nil, fmt.Errorf("offline: неизвестная версия файла %d", f.Version)
	}
	sealed := &Sealed{body: f.Body, mac: f.MAC}
	if err := json.Unmarshal(f.Body, &sealed.snapshot); err != nil {
		return nil, fmt.Errorf("offline: файл %s повреждён: %w", path, err)
	}
	return sealed, nil
}

// Save подписывает snapshot ключом из masterKey и атомарно записывает в path
// с правами 0600.
func Save(path string, masterKey []byte, snapshot *Snapshot) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("offline: %w", err)
	}
	mac, err := sign(masterKey, body)
	if err != nil {
		return err
	}
	data, err := json.Marshal(file{Version: version, Body: body, MAC: mac})
	if err != nil {
		return fmt.Errorf("offline: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("offline: %w", err)
	}
	tmp, err := os.CreateTemp(dir, FileName+".*")
	if err != nil {
		return fmt.Errorf("offline: %w", err)
	}
	defer os.Remove(tmp.Name()) // после успешного переименования файла уже нет
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("offline: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("offline: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("offline: %w", err)
	}
	return nil
}

// Remove удаляет копию; отсутствие файла — не ошибка.
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("offline: %w", err)
	}
	return nil
}

// Archive откладывает копию path в path+StaleSuffix (заменяя прежнюю отложенную);
// отсутствие файла — не ошибка.
func Archive(path string) error {
	if err := os.Rename(path, path+StaleSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("offline: %w", err)
	}
	return nil
}

// sign возвращает HMAC-SHA256 тела ключом HKDF(masterKey, "offline-cache").
func sign(masterKey, body []byte) ([]byte, error) {
	key, err := crypto.HKDF(masterKey, crypto.InfoOfflineCache)
	if err != nil {
		return nil, fmt.Errorf("offline: %w", err)
	}
	defer clear(key)
	m := hmac.New(sha256.New, key)
	m.Write(body)
	return m.Sum(nil), nil
}
