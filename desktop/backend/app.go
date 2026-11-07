// Package backend — логика десктоп-приложения Shron: методы, которые Wails
// предоставляет интерфейсу. Работа с сервером и ключами — в internal/client/vault;
// здесь настройки, преобразование данных для интерфейса, диалоги и буфер обмена.
package backend

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/F3dosik/Shron/internal/client/config"
	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
	pb "github.com/F3dosik/Shron/proto/gen"
)

// Значения по умолчанию для Options.
const (
	defaultClipboardClearAfter = 30 * time.Second
	// maxCACertSize — ограничение на размер выбранного файла сертификата.
	maxCACertSize = 64 << 10
)

// Options — параметры App.
type Options struct {
	// Dir — каталог данных: настройки, CA-сертификат, сессия.
	Dir string
	// UI — оболочка приложения.
	UI UI
	// Insecure — соединение без TLS (только для тестов и локальной разработки).
	Insecure bool
	// ClipboardClearAfter — через сколько очищать скопированное; 0 — 30 секунд.
	ClipboardClearAfter time.Duration
}

// State — текущее состояние приложения для интерфейса.
type State struct {
	// Configured — задан адрес сервера.
	Configured      bool   `json:"configured"`
	ServerAddress   string `json:"serverAddress"`
	HasCACert       bool   `json:"hasCaCert"`
	AutoLockMinutes int    `json:"autoLockMinutes"`
	// Login — текущая учётка; пустой — нужен вход.
	Login string `json:"login"`
	// Unlocked — хранилище разблокировано.
	Unlocked bool `json:"unlocked"`
	// AdminMode — приложение подключено к административному порту сервера: доступна
	// только административная панель, хранилища там нет.
	AdminMode bool `json:"adminMode"`
}

// App — методы приложения для интерфейса. Все методы безопасны для одновременного вызова.
type App struct {
	opts Options

	mu       sync.Mutex
	ctx      context.Context
	settings Settings
	vault    *vault.Vault
	admin    *adminConn

	clipMu  sync.Mutex
	clipGen uint64
}

// NewApp создаёт App. Настройки загружаются в Startup.
func NewApp(opts Options) *App {
	if opts.ClipboardClearAfter <= 0 {
		opts.ClipboardClearAfter = defaultClipboardClearAfter
	}
	return &App{opts: opts, ctx: context.Background()}
}

// DefaultDir возвращает каталог данных приложения в профиле пользователя
// (~/.config/Shron, %AppData%\Shron, ~/Library/Application Support/Shron).
// Каталог прежней версии (GophKeeper) переносится на новое место.
func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "Shron")
	if err := config.MigrateDir(filepath.Join(dir, "GophKeeper"), path); err != nil {
		return "", err
	}
	return path, nil
}

// Startup загружает настройки и, если сервер задан, подключается к нему.
// Вызывается оболочкой при запуске. Это функция, а не метод App: Wails делает
// доступными интерфейсу все экспортируемые методы, а этот вызывать оттуда нельзя.
func Startup(a *App, ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx

	settings, err := loadSettings(a.opts.Dir)
	if err != nil {
		return err
	}
	a.settings = settings
	if settings.ServerAddress == "" {
		return nil
	}
	return a.openVaultLocked()
}

// Shutdown блокирует хранилище и закрывает соединение. Вызывается при закрытии окна.
func Shutdown(a *App) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeAdminLocked()
	if a.vault != nil {
		_ = a.vault.Close()
		a.vault = nil
	}
}

// GetState возвращает текущее состояние.
func (a *App) GetState() State {
	a.mu.Lock()
	defer a.mu.Unlock()

	state := State{
		Configured:      a.settings.ServerAddress != "",
		ServerAddress:   a.settings.ServerAddress,
		AutoLockMinutes: a.settings.AutoLockMinutes,
	}
	if ca, err := loadCACert(a.opts.Dir); err == nil && len(ca) > 0 {
		state.HasCACert = true
	}
	if a.vault != nil {
		state.Login = a.vault.CurrentLogin()
		state.Unlocked = a.vault.IsUnlocked()
	}
	state.AdminMode = a.admin != nil && a.admin.active
	return state
}

// ChooseCACert показывает диалог выбора сертификата и возвращает его содержимое (PEM).
// Пустая строка — пользователь отменил выбор.
func (a *App) ChooseCACert() (string, error) {
	path, err := a.opts.UI.OpenFile("Корневой сертификат сервера (ca.crt)", []FileFilter{
		{DisplayName: "Сертификаты (*.crt, *.pem)", Pattern: "*.crt;*.pem"},
	})
	if err != nil || path == "" {
		return "", toUIError(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", toUIError(err)
	}
	if info.Size() > maxCACertSize {
		return "", validationError("Файл слишком большой для сертификата")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", toUIError(err)
	}
	if err := checkCACert(data); err != nil {
		return "", err
	}
	return string(data), nil
}

// CheckServer проверяет, что сервер доступен по адресу address и его сертификат
// подписан caPEM (пустой — системные корневые сертификаты). useSavedCA — проверить
// с сохранённым ранее сертификатом: интерфейс не хранит его содержимое.
func (a *App) CheckServer(address, caPEM string, useSavedCA bool) (string, error) {
	address, err := normalizeAddress(address)
	if err != nil {
		return "", err
	}
	ca, err := a.resolveCACert(caPEM, useSavedCA)
	if err != nil {
		return "", err
	}
	conn, err := grpcclient.DialWithOptions(address, grpcclient.DialOptions{
		CACertPEM: ca,
		Insecure:  a.opts.Insecure,
	})
	if err != nil {
		return "", toUIError(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(a.context(), 10*time.Second)
	defer cancel()
	_, kdf, err := grpcclient.NewAuthClient(pb.NewAuthClient(conn)).GetSalt(ctx, "connection-check")
	if err != nil {
		return "", toUIError(err)
	}
	return fmt.Sprintf("Сервер доступен (Argon2id для новых учёток: t=%d, %d MiB)", kdf.Time, kdf.MemoryKiB/1024), nil
}

// SaveSettings сохраняет настройки и переподключается к серверу. Хранилище при этом
// блокируется; логин сохраняется, если сервер не менялся. useSavedCA — оставить
// сохранённый сертификат; иначе сохраняется caPEM (пустой — системные сертификаты).
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
	settings := Settings{ServerAddress: address, AutoLockMinutes: autoLockMinutes}
	if err := saveSettings(a.opts.Dir, settings, ca); err != nil {
		return toUIError(err)
	}
	if settings.ServerAddress != a.settings.ServerAddress {
		// Сессия принадлежит другому серверу.
		_ = os.Remove(filepath.Join(a.opts.Dir, sessionFile))
	}
	a.settings = settings
	return a.openVaultLocked()
}

// Register регистрирует учётку с параметрами Argon2id по умолчанию.
func (a *App) Register(login, password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	if err := checkCredentials(login, password); err != nil {
		return err
	}
	return registrationError(v.Register(a.context(), strings.TrimSpace(login), password, domain.DefaultKDFParams))
}

// registrationError переводит ошибку регистрации. Сервер отказывает с PermissionDenied,
// когда регистрация на нём закрыта: учётки тогда создаёт администратор.
func registrationError(err error) error {
	if vault.ErrorCode(err) == vault.CodePermissionDenied {
		return &Error{Code: vault.CodePermissionDenied, Message: registrationClosedMessage}
	}
	return toUIError(err)
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
	changeRequired, err := v.SignIn(a.context(), strings.TrimSpace(login), password)
	return changeRequired, toUIError(err)
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
	return toUIError(v.CompletePasswordChange(a.context(), temporaryPassword, newPassword))
}

// Unlock разблокирует хранилище паролем текущей учётки.
func (a *App) Unlock(password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toUIError(v.Unlock(a.context(), password))
}

// AcceptKDFChange забывает запомненные на устройстве параметры Argon2id учётки login.
// Интерфейс вызывает его после ошибки KDF_DOWNGRADE, когда пользователь подтвердил, что
// сам сменил пароль с более слабыми параметрами; затем вход нужно повторить.
func (a *App) AcceptKDFChange(login string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toUIError(v.ForgetKDFParams(strings.TrimSpace(login)))
}

// Lock блокирует хранилище.
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
	return toUIError(v.ChangePassword(a.context(), oldPassword, newPassword, kdf))
}

// Logout выходит из учётки; allSessions — на всех устройствах.
func (a *App) Logout(allSessions bool) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toUIError(v.Logout(a.context(), allSessions))
}

// DeleteAccount безвозвратно удаляет учётку со всеми секретами.
func (a *App) DeleteAccount(password string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	return toUIError(v.DeleteAccount(a.context(), password))
}

// CopyToClipboard копирует значение и очищает буфер через ClipboardClearAfter,
// если за это время в него не скопировали что-то другое.
func (a *App) CopyToClipboard(value string) error {
	if err := a.opts.UI.ClipboardSet(value); err != nil {
		return toUIError(err)
	}

	a.clipMu.Lock()
	a.clipGen++
	gen := a.clipGen
	a.clipMu.Unlock()

	time.AfterFunc(a.opts.ClipboardClearAfter, func() {
		a.clipMu.Lock()
		defer a.clipMu.Unlock()
		if a.clipGen != gen {
			return
		}
		if current, err := a.opts.UI.ClipboardGet(); err == nil && current == value {
			_ = a.opts.UI.ClipboardSet("")
		}
	})
	return nil
}

// currentVault возвращает подключённое хранилище или ошибку «не настроено».
func (a *App) currentVault() (*vault.Vault, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault == nil {
		return nil, &Error{Code: CodeNotConfigured, Message: "Укажите адрес сервера в настройках"}
	}
	return a.vault, nil
}

// context возвращает контекст приложения.
func (a *App) context() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx
}

// openVaultLocked (пере)подключается к серверу из настроек. Вызывается под a.mu.
func (a *App) openVaultLocked() error {
	if a.vault != nil {
		_ = a.vault.Close()
		a.vault = nil
	}

	caPEM, err := loadCACert(a.opts.Dir)
	if err != nil {
		return toUIError(err)
	}
	v, err := vault.Open(vault.Config{
		ServerAddress: a.settings.ServerAddress,
		CACertPEM:     caPEM,
		Insecure:      a.opts.Insecure,
		SessionPath:   filepath.Join(a.opts.Dir, sessionFile),
		AutoLockAfter: time.Duration(a.settings.AutoLockMinutes) * time.Minute,
		OnLock: func(reason vault.LockReason) {
			a.opts.UI.Emit(EventLocked, string(reason))
		},
	})
	if err != nil {
		return toUIError(err)
	}
	a.vault = v
	a.openAdminLocked(caPEM)
	return nil
}

// resolveCACert возвращает сертификат для подключения: сохранённый (useSavedCA)
// или переданный, проверив, что это PEM.
func (a *App) resolveCACert(caPEM string, useSavedCA bool) ([]byte, error) {
	if useSavedCA {
		ca, err := loadCACert(a.opts.Dir)
		if err != nil {
			return nil, toUIError(err)
		}
		return ca, nil
	}
	if caPEM == "" {
		return nil, nil
	}
	if err := checkCACert([]byte(caPEM)); err != nil {
		return nil, err
	}
	return []byte(caPEM), nil
}

// normalizeAddress приводит адрес сервера к host:port; без порта — порт по умолчанию.
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

// minPasswordLength — минимальная длина мастер-пароля (как в CLI).
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
	if timeCost <= 0 || memoryMiB <= 0 || memoryMiB > 1<<20 {
		return domain.KDFParams{}, validationError("Параметры Argon2id: проходов 1–10, память 19–1024 MiB")
	}
	params := domain.KDFParams{
		Time:      uint32(timeCost),
		MemoryKiB: uint32(memoryMiB) * 1024,
		Threads:   domain.DefaultKDFParams.Threads,
	}
	if err := params.Validate(); err != nil {
		return domain.KDFParams{}, validationError("Параметры Argon2id: проходов 1–10, память 19–1024 MiB")
	}
	return params, nil
}
