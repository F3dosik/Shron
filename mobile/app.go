// Package mobile — клиент Shron для мобильных приложений, собирается в Android-библиотеку
// через gomobile bind (make android-aar). Работа с сервером и ключами — в internal/client/vault;
// здесь настройки и преобразование данных в типы, которые понимает gomobile.
//
// Ограничения gomobile определяют вид API:
//   - параметры и результаты — строки, числа, bool, []byte и указатели на структуры пакета;
//   - списков нет, поэтому секреты возвращаются в *SecretList с методами Len и Get;
//   - функция NewApp в Kotlin — конструктор App(dataDir, insecure, listener);
//   - ошибка в Kotlin — исключение, текст которого — JSON {"code": ..., "message": ...}
//     (см. Error): по коду приложение решает, что показать, сообщение — для пользователя.
//
// Все методы блокирующие (сеть, Argon2id с 64 MiB памяти), поэтому вызывать их нужно не
// из главного потока, например в withContext(Dispatchers.IO). Методы безопасны для
// одновременного вызова.
package mobile

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
	pb "github.com/F3dosik/Shron/proto/gen"
)

// checkServerTimeout — таймаут проверки соединения с сервером.
const checkServerTimeout = 10 * time.Second

// LockListener получает уведомления о блокировке хранилища. Реализуется в Kotlin.
// OnLock вызывается из фонового потока Go, поэтому обновлять интерфейс нужно через
// главный поток. reason — LockManual, LockIdle, LockSessionExpired или LockSignedOut.
type LockListener interface {
	OnLock(reason string)
}

// Причины блокировки для LockListener.
const (
	LockManual         = string(vault.LockManual)
	LockIdle           = string(vault.LockIdle)
	LockSessionExpired = string(vault.LockSessionExpired)
	LockSignedOut      = string(vault.LockSignedOut)
)

// State — текущее состояние приложения.
type State struct {
	// Configured — задан адрес сервера.
	Configured      bool
	ServerAddress   string
	HasCACert       bool
	AutoLockMinutes int
	// Login — текущая учётка; пустой — нужен вход.
	Login string
	// Unlocked — хранилище разблокировано.
	Unlocked bool
	// Offline — сервер недоступен, записи показаны из копии на устройстве (только чтение).
	Offline bool
	// SyncedAt — когда копия на устройстве последний раз получила записи с сервера,
	// миллисекунды Unix; 0 — копии нет.
	SyncedAt int64
	// PendingChanges — изменения, сделанные без сети и ещё не отправленные;
	// Conflicts — сколько из них ждут решения пользователя (см. Conflicts).
	PendingChanges int
	Conflicts      int
}

// App — клиент Shron.
type App struct {
	dir      string
	insecure bool
	listener LockListener

	mu       sync.Mutex
	settings settings
	vault    *vault.Vault
	// openErr — ошибка последнего подключения к серверу из настроек; возвращается
	// методами, которым нужен сервер, пока настройки не исправлены.
	openErr error
}

// NewApp загружает настройки из dataDir и, если сервер задан, подключается к нему.
// Ошибка подключения не мешает созданию App: её вернут методы, которым нужен сервер,
// а исправить настройки можно через SaveSettings.
// dataDir — каталог приложения (на Android — context.filesDir), в нём хранятся настройки,
// CA-сертификат и сессия. insecure — соединение без TLS, только для разработки (например,
// с эмулятора к серверу на компьютере). listener может быть nil.
func NewApp(dataDir string, insecure bool, listener LockListener) (*App, error) {
	if dataDir == "" {
		return nil, validationError("Не задан каталог данных приложения")
	}
	s, err := loadSettings(dataDir)
	if err != nil {
		return nil, toError(err)
	}
	a := &App{dir: dataDir, insecure: insecure, listener: listener, settings: s}
	if s.ServerAddress != "" {
		_ = a.openVaultLocked()
	}
	return a, nil
}

// Close блокирует хранилище и закрывает соединение. Вызывается при завершении приложения.
func (a *App) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault != nil {
		_ = a.vault.Close()
		a.vault = nil
	}
}

// GetState возвращает текущее состояние.
func (a *App) GetState() *State {
	a.mu.Lock()
	defer a.mu.Unlock()

	state := &State{
		Configured:      a.settings.ServerAddress != "",
		ServerAddress:   a.settings.ServerAddress,
		AutoLockMinutes: a.settings.AutoLockMinutes,
	}
	if ca, err := loadCACert(a.dir); err == nil && len(ca) > 0 {
		state.HasCACert = true
	}
	if a.vault != nil {
		state.Login = a.vault.CurrentLogin()
		state.Unlocked = a.vault.IsUnlocked()
		offline, syncedAt := a.vault.Offline()
		state.Offline = offline
		state.SyncedAt = unixMilli(syncedAt)
		state.PendingChanges, state.Conflicts = a.vault.PendingChanges()
	}
	return state
}

// CheckServer проверяет, что сервер доступен по адресу address и его сертификат подписан
// caPEM (пустой — системные корневые сертификаты). useSavedCA — проверить с сохранённым
// ранее сертификатом. Возвращает сообщение для пользователя.
func (a *App) CheckServer(address, caPEM string, useSavedCA bool) (string, error) {
	address, err := normalizeAddress(address)
	if err != nil {
		return "", err
	}
	ca, err := a.resolveCACert(caPEM, useSavedCA)
	if err != nil {
		return "", err
	}
	conn, err := grpcclient.DialWithOptions(address, grpcclient.DialOptions{CACertPEM: ca, Insecure: a.insecure})
	if err != nil {
		return "", toError(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), checkServerTimeout)
	defer cancel()
	_, kdf, err := grpcclient.NewAuthClient(pb.NewAuthClient(conn)).GetSalt(ctx, "connection-check")
	if err != nil {
		return "", toError(err)
	}
	return fmt.Sprintf("Сервер доступен (Argon2id для новых учёток: t=%d, %d MiB)", kdf.Time, kdf.MemoryKiB/1024), nil
}

// SaveSettings сохраняет настройки и переподключается к серверу. Хранилище при этом
// блокируется; логин сохраняется, если сервер не менялся. useSavedCA — оставить
// сохранённый сертификат; иначе сохраняется caPEM (пустой — системные сертификаты).
// autoLockMinutes — блокировка после бездействия, 0 — не блокировать.
func (a *App) SaveSettings(address, caPEM string, useSavedCA bool, autoLockMinutes int) error {
	address, err := normalizeAddress(address)
	if err != nil {
		return err
	}
	ca, err := a.resolveCACert(caPEM, useSavedCA)
	if err != nil {
		return err
	}
	if autoLockMinutes < 0 || autoLockMinutes > 24*60 {
		return validationError("Автоблокировка — от 0 до 1440 минут")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	s := settings{ServerAddress: address, AutoLockMinutes: autoLockMinutes}
	if err := saveSettings(a.dir, s, ca); err != nil {
		return toError(err)
	}
	if s.ServerAddress != a.settings.ServerAddress {
		// Сессия и копия хранилища принадлежат другому серверу.
		_ = os.Remove(filepath.Join(a.dir, sessionFile))
		_ = offline.Remove(filepath.Join(a.dir, offline.FileName))
	}
	a.settings = s
	return a.openVaultLocked()
}

// Register регистрирует учётку с параметрами Argon2id по умолчанию. Вход — отдельно (SignIn).
func (a *App) Register(login, password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	if err := checkCredentials(login, password); err != nil {
		return err
	}
	return registrationError(v.Register(context.Background(), strings.TrimSpace(login), password, domain.DefaultKDFParams))
}

// registrationError переводит ошибку регистрации. Сервер отказывает с PermissionDenied,
// когда регистрация на нём закрыта: учётки тогда создаёт администратор.
func registrationError(err error) error {
	if vault.ErrorCode(err) == vault.CodePermissionDenied {
		return &Error{Code: CodePermissionDenied, Message: registrationClosedMessage}
	}
	return toError(err)
}

const registrationClosedMessage = "Регистрация на этом сервере закрыта. Обратитесь к администратору сервера: " +
	"он может создать аккаунт и выдать временный пароль."

// SignIn входит в учётку. Возвращает true, если пароль временный и его нужно сменить
// (CompletePasswordChange); иначе хранилище разблокировано.
func (a *App) SignIn(login, password string) (bool, error) {
	v, err := a.currentVault()
	if err != nil {
		return false, err
	}
	changeRequired, err := v.SignIn(context.Background(), strings.TrimSpace(login), password)
	return changeRequired, toError(err)
}

// CompletePasswordChange заменяет временный пароль постоянным и разблокирует хранилище.
func (a *App) CompletePasswordChange(temporaryPassword, newPassword string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	if err := checkNewPassword(temporaryPassword, newPassword); err != nil {
		return err
	}
	return toError(v.CompletePasswordChange(context.Background(), temporaryPassword, newPassword))
}

// Unlock разблокирует хранилище паролем текущей учётки.
func (a *App) Unlock(password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.Unlock(context.Background(), password))
}

// UnlockWithKey разблокирует хранилище текущей учётки мастер-ключом, полученным раньше
// из MasterKey (на Android — расшифрованным ключом Keystore после биометрии). По ключу
// выполняется вход на сервер; без сервера ключ проверяется по копии на устройстве. Ошибка CodeKeyRejected — пароль сменили, ключ
// больше не годится: его нужно удалить и разблокировать паролем.
func (a *App) UnlockWithKey(masterKey []byte) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.UnlockWithKey(context.Background(), masterKey))
}

// MasterKey возвращает мастер-ключ разблокированного хранилища, чтобы сохранить его для
// UnlockWithKey. Хранить его можно только зашифрованным ключом Keystore, а копию в
// памяти стереть сразу после шифрования.
func (a *App) MasterKey() ([]byte, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	key, err := v.MasterKey()
	return key, toError(err)
}

// AcceptKDFChange забывает запомненные на устройстве параметры Argon2id учётки login.
// Вызывается после ошибки CodeKDFDowngrade, когда пользователь подтвердил, что сам сменил
// пароль с более слабыми параметрами на другом устройстве; затем вход нужно повторить.
func (a *App) AcceptKDFChange(login string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.ForgetKDFParams(strings.TrimSpace(login)))
}

// RetryServer просит при следующем чтении обратиться к серверу, даже если он недавно
// не ответил: без этого после сбоя сети записи полминуты читаются из копии на
// устройстве. Вызывается, когда пользователь обновляет список вручную.
func (a *App) RetryServer() {
	if v, err := a.currentVault(); err == nil {
		v.RetryServer()
	}
}

// Lock блокирует хранилище. Приложение вызывает его, например, при уходе в фон.
func (a *App) Lock() {
	if v, err := a.currentVault(); err == nil {
		v.Lock()
	}
}

// ChangePassword меняет мастер-пароль с перешифровкой всех секретов.
// kdfTime и kdfMemoryMiB — параметры Argon2id для нового пароля.
func (a *App) ChangePassword(oldPassword, newPassword string, kdfTime, kdfMemoryMiB int) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	if err := checkNewPassword(oldPassword, newPassword); err != nil {
		return err
	}
	kdf, err := kdfParams(kdfTime, kdfMemoryMiB)
	if err != nil {
		return err
	}
	return toError(v.ChangePassword(context.Background(), oldPassword, newPassword, kdf))
}

// Logout выходит из учётки; allSessions — на всех устройствах.
func (a *App) Logout(allSessions bool) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.Logout(context.Background(), allSessions))
}

// DeleteAccount безвозвратно удаляет учётку со всеми секретами.
func (a *App) DeleteAccount(password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toError(v.DeleteAccount(context.Background(), password))
}

// currentVault возвращает подключённое хранилище или ошибку «не настроено».
func (a *App) currentVault() (*vault.Vault, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.openErr != nil {
		return nil, a.openErr
	}
	if a.vault == nil {
		return nil, &Error{Code: CodeNotConfigured, Message: "Укажите адрес сервера в настройках"}
	}
	return a.vault, nil
}

// openVaultLocked (пере)подключается к серверу из настроек и запоминает ошибку в openErr.
// Вызывается под a.mu (или до того, как App стал доступен другим потокам).
func (a *App) openVaultLocked() error {
	a.openErr = a.dialVaultLocked()
	return a.openErr
}

// dialVaultLocked закрывает прежнее соединение и открывает новое. Вызывается под a.mu.
func (a *App) dialVaultLocked() error {
	if a.vault != nil {
		_ = a.vault.Close()
		a.vault = nil
	}

	caPEM, err := loadCACert(a.dir)
	if err != nil {
		return toError(err)
	}
	v, err := vault.Open(vault.Config{
		ServerAddress: a.settings.ServerAddress,
		CACertPEM:     caPEM,
		Insecure:      a.insecure,
		SessionPath:   filepath.Join(a.dir, sessionFile),
		CachePath:     filepath.Join(a.dir, offline.FileName),
		OfflineWrites: true,
		AutoLockAfter: time.Duration(a.settings.AutoLockMinutes) * time.Minute,
		OnLock: func(reason vault.LockReason) {
			if a.listener != nil {
				a.listener.OnLock(string(reason))
			}
		},
	})
	if err != nil {
		return toError(err)
	}
	a.vault = v
	return nil
}

// resolveCACert возвращает сертификат для подключения: сохранённый (useSavedCA)
// или переданный, проверив, что это PEM.
func (a *App) resolveCACert(caPEM string, useSavedCA bool) ([]byte, error) {
	if useSavedCA {
		ca, err := loadCACert(a.dir)
		if err != nil {
			return nil, toError(err)
		}
		return ca, nil
	}
	if strings.TrimSpace(caPEM) == "" {
		return nil, nil
	}
	if err := checkCACert([]byte(caPEM)); err != nil {
		return nil, err
	}
	return []byte(caPEM), nil
}

func normalizeAddress(address string) (string, error) {
	normalized, err := grpcclient.NormalizeAddress(address)
	if err != nil {
		return "", validationError("Адрес сервера — имя или IP, порт необязателен: 192.168.1.5 или 192.168.1.5:" +
			grpcclient.DefaultPort)
	}
	return normalized, nil
}

// checkCACert проверяет, что в данных есть хотя бы один PEM-сертификат.
func checkCACert(pem []byte) error {
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		return validationError("Файл не содержит сертификата в формате PEM")
	}
	return nil
}

// minPasswordLength — минимальная длина мастер-пароля (как в CLI и десктопе).
const minPasswordLength = 8

// checkCredentials проверяет логин и пароль новой учётки.
func checkCredentials(login, password string) error {
	if strings.TrimSpace(login) == "" {
		return validationError("Укажите логин")
	}
	if len(password) < minPasswordLength {
		return validationError(fmt.Sprintf("Пароль должен быть не короче %d символов", minPasswordLength))
	}
	return nil
}

// checkNewPassword проверяет новый пароль при смене.
func checkNewPassword(oldPassword, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return validationError(fmt.Sprintf("Пароль должен быть не короче %d символов", minPasswordLength))
	}
	if newPassword == oldPassword {
		return validationError("Новый пароль совпадает с текущим")
	}
	return nil
}

// kdfParams собирает и проверяет параметры Argon2id.
func kdfParams(timeCost, memoryMiB int) (domain.KDFParams, error) {
	const message = "Параметры Argon2id: проходов 1–10, память 19–1024 MiB"
	if timeCost <= 0 || memoryMiB <= 0 || memoryMiB > 1<<20 {
		return domain.KDFParams{}, validationError(message)
	}
	params := domain.KDFParams{
		Time:      uint32(timeCost),
		MemoryKiB: uint32(memoryMiB) * 1024,
		Threads:   domain.DefaultKDFParams.Threads,
	}
	if err := params.Validate(); err != nil {
		return domain.KDFParams{}, validationError(message)
	}
	return params, nil
}
