// Package vault — контроллер клиента для приложений с интерфейсом (десктоп, мобильное).
//
// В отличие от CLI, где каждая команда спрашивает пароль и заново выводит ключи,
// Vault выводит мастер-ключ один раз при разблокировке и держит его в памяти до
// блокировки: вручную, по таймауту бездействия или когда сервер перестаёт принимать
// токен. Токен обновляется по мастер-ключу без пароля, но только после успешного
// запроса: если токен отозван (выход на всех устройствах, смена пароля), запрос
// не проходит, хранилище блокируется и требует пароль.
//
// Все методы безопасны для вызова из разных горутин; операции выполняются по одной.
package vault

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/client/kdfpin"
	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/client/session"
	"github.com/F3dosik/Shron/internal/domain"
	pb "github.com/F3dosik/Shron/proto/gen"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

// Значения по умолчанию для Config.
const (
	defaultRequestTimeout = 30 * time.Second
	defaultRefreshMargin  = 10 * time.Minute
	// bulkTimeout — для операций над всеми секретами (смена пароля, экспорт).
	bulkTimeout = 5 * time.Minute
)

// Config — параметры Vault.
type Config struct {
	// ServerAddress — адрес сервера (host:port).
	ServerAddress string
	// CACertPEM — корневой сертификат сервера; пустой — системные корневые сертификаты.
	CACertPEM []byte
	// Insecure разрешает соединение без TLS (только для локальной разработки).
	Insecure bool
	// SessionPath — файл сессии (логин и токен).
	SessionPath string
	// AutoLockAfter — блокировка после бездействия; 0 — не блокировать автоматически.
	AutoLockAfter time.Duration
	// RequestTimeout — таймаут одной операции; 0 — 30 секунд.
	RequestTimeout time.Duration
	// RefreshMargin — токен обновляется после успешного запроса, если до его истечения
	// осталось меньше этого времени; 0 — 10 минут.
	RefreshMargin time.Duration
	// OnLock вызывается после каждой блокировки (вне внутренних блокировок Vault,
	// поэтому из него можно вызывать методы Vault). Может быть nil.
	OnLock func(reason LockReason)
}

// LockReason — причина блокировки хранилища.
type LockReason string

const (
	// LockManual — блокировка по вызову Lock.
	LockManual LockReason = "manual"
	// LockIdle — блокировка по таймауту бездействия.
	LockIdle LockReason = "idle"
	// LockSessionExpired — сервер не принял токен: он истёк или отозван.
	LockSessionExpired LockReason = "session_expired"
	// LockSignedOut — выход из учётки или её удаление.
	LockSignedOut LockReason = "signed_out"
)

// SecretSummary — сведения о секрете для списка, без его содержимого.
type SecretSummary struct {
	Name      string
	Type      domain.SecretType
	Metadata  string
	Group     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Vault — клиентское хранилище с состоянием разблокировки.
type Vault struct {
	cfg    Config
	conn   *grpc.ClientConn
	tokens *grpcclient.TokenStore
	auth   service.AuthService
	client grpcclient.SecretsClient

	mu        sync.Mutex
	login     string
	masterKey []byte
	secrets   service.SecretsService
	idleTimer *time.Timer
	idleGen   uint64
}

// Open подключается к серверу и восстанавливает логин и токен из файла сессии,
// если он есть. Хранилище открывается заблокированным.
func Open(cfg Config) (*Vault, error) {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.RefreshMargin <= 0 {
		cfg.RefreshMargin = defaultRefreshMargin
	}

	var login, token string
	sess, err := session.Load(cfg.SessionPath)
	switch {
	case err == nil:
		login, token = sess.Login, sess.Token
	case !errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("vault: load session: %w", err)
	}

	tokens := grpcclient.NewTokenStore(token)
	conn, err := grpcclient.DialWithOptions(cfg.ServerAddress, grpcclient.DialOptions{
		CACertPEM: cfg.CACertPEM,
		Insecure:  cfg.Insecure,
		Tokens:    tokens,
	})
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}

	return &Vault{
		cfg:    cfg,
		conn:   conn,
		tokens: tokens,
		auth: service.NewAuthService(grpcclient.NewAuthClient(pb.NewAuthClient(conn)), cfg.SessionPath, tokens,
			service.WithKDFPins(kdfpin.NextTo(cfg.SessionPath), cfg.ServerAddress)),
		client: grpcclient.NewSecretsClient(pb.NewSecretsClient(conn)),
		login:  login,
	}, nil
}

// Close блокирует хранилище и закрывает соединение.
func (v *Vault) Close() error {
	v.mu.Lock()
	v.lockLocked()
	v.mu.Unlock()
	return v.conn.Close()
}

// CurrentLogin возвращает логин текущей учётки или пустую строку, если вход не выполнен.
func (v *Vault) CurrentLogin() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.login
}

// IsUnlocked сообщает, разблокировано ли хранилище.
func (v *Vault) IsUnlocked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.secrets != nil
}

// Register регистрирует учётку. Вход после регистрации выполняется отдельно (SignIn).
func (v *Vault) Register(ctx context.Context, login, password string, kdf domain.KDFParams) error {
	ctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()
	return v.auth.CreateUser(ctx, login, password, kdf)
}

// SignIn входит в учётку и разблокирует хранилище. Если пароль временный, возвращает
// passwordChangeRequired == true: хранилище остаётся заблокированным, а получить доступ
// можно только через CompletePasswordChange.
func (v *Vault) SignIn(ctx context.Context, login, password string) (passwordChangeRequired bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()

	v.mu.Lock()
	v.lockLocked()
	masterKey, err := v.auth.Unlock(ctx, login, password)
	if errors.Is(err, domain.ErrPasswordChangeRequired) {
		// Unlock не сохраняет токен временного пароля; Login получает его для смены пароля.
		_, err = v.auth.Login(ctx, login, password)
		if err == nil {
			v.login = login
		}
		v.mu.Unlock()
		return err == nil, wrapCredentials(err, ErrWrongPassword)
	}
	if err != nil {
		v.mu.Unlock()
		return false, wrapCredentials(err, ErrWrongPassword)
	}
	err = v.unlockLocked(login, masterKey)
	v.mu.Unlock()
	return false, err
}

// ForgetKDFParams забывает параметры Argon2id учётки login, запомненные на этом
// устройстве. Вызывается, когда вход отклонён с CodeKDFDowngrade, а пользователь
// подтвердил, что сам сменил пароль с более слабыми параметрами на другом устройстве.
func (v *Vault) ForgetKDFParams(login string) error {
	return v.auth.ForgetKDFParams(login)
}

// CompletePasswordChange задаёт постоянный пароль вместо временного (после SignIn,
// вернувшего passwordChangeRequired) и разблокирует хранилище.
func (v *Vault) CompletePasswordChange(ctx context.Context, temporaryPassword, newPassword string) error {
	login := v.CurrentLogin()
	if login == "" {
		return ErrNotSignedIn
	}
	ctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()

	err := v.auth.ChangePassword(ctx, login, temporaryPassword, newPassword, domain.DefaultKDFParams, nil)
	if err != nil {
		return wrapCredentials(err, ErrWrongPassword)
	}
	_, err = v.SignIn(ctx, login, newPassword)
	return err
}

// Unlock разблокирует хранилище паролем текущей учётки (после блокировки или запуска).
func (v *Vault) Unlock(ctx context.Context, password string) error {
	v.mu.Lock()
	login := v.login
	v.mu.Unlock()
	if login == "" {
		return ErrNotSignedIn
	}
	_, err := v.SignIn(ctx, login, password)
	return err
}

// UnlockWithKey разблокирует хранилище текущей учётки мастер-ключом, сохранённым после
// входа по паролю (MasterKey), — например, под защитой биометрии. Argon2id не нужен,
// но сервер нужен: по ключу выполняется вход, так что отозванная сессия или сменённый
// пароль ключ не пропустят. Если сервер ключ не принял, возвращается ErrKeyRejected.
// masterKey копируется; вызывающий может стереть свой экземпляр сразу после вызова.
func (v *Vault) UnlockWithKey(ctx context.Context, masterKey []byte) error {
	ctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()

	key := bytes.Clone(masterKey)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.login == "" {
		clear(key)
		return ErrNotSignedIn
	}
	v.lockLocked()
	if err := v.auth.RefreshToken(ctx, v.login, key); err != nil {
		clear(key)
		if errors.Is(err, domain.ErrPasswordChangeRequired) {
			return ErrKeyRejected
		}
		return wrapCredentials(err, ErrKeyRejected)
	}
	return v.unlockLocked(v.login, key)
}

// MasterKey возвращает копию мастер-ключа разблокированного хранилища, чтобы
// сохранить его для UnlockWithKey. Ключ даёт полный доступ к секретам учётки: хранить
// его можно только зашифрованным, например ключом Android Keystore.
func (v *Vault) MasterKey() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.secrets == nil {
		return nil, ErrLocked
	}
	return bytes.Clone(v.masterKey), nil
}

// Lock блокирует хранилище: ключи стираются, для работы снова нужен пароль.
func (v *Vault) Lock() {
	v.mu.Lock()
	locked := v.lockLocked()
	v.mu.Unlock()
	if locked {
		v.notifyLock(LockManual)
	}
}

// ListSecrets возвращает список секретов без их содержимого.
func (v *Vault) ListSecrets(ctx context.Context) ([]SecretSummary, error) {
	var result []SecretSummary
	err := v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		infos, err := svc.ListSecrets(ctx)
		if err != nil {
			return err
		}
		result = make([]SecretSummary, 0, len(infos))
		for _, info := range infos {
			result = append(result, SecretSummary{
				Name:      info.Name,
				Type:      info.Type,
				Metadata:  info.Metadata,
				Group:     info.Group,
				CreatedAt: info.CreatedAt,
				UpdatedAt: info.UpdatedAt,
			})
		}
		return nil
	})
	return result, err
}

// ExportSecrets возвращает все секреты с содержимым — для экспорта хранилища в файл.
func (v *Vault) ExportSecrets(ctx context.Context) ([]domain.SecretPayload, error) {
	var result []domain.SecretPayload
	err := v.withSecrets(ctx, bulkTimeout, func(ctx context.Context, svc service.SecretsService) error {
		infos, err := svc.ListSecrets(ctx)
		if err != nil {
			return err
		}
		result = make([]domain.SecretPayload, 0, len(infos))
		for _, info := range infos {
			result = append(result, info.SecretPayload)
		}
		return nil
	})
	return result, err
}

// LoginMatch — логин, подходящий к заполняемой форме.
type LoginMatch struct {
	Name     string
	Login    string
	Password string
}

// MatchLogins возвращает логины, у которых среди сайтов и приложений есть подходящий
// к форме на сайте webDomain или в приложении packageName (domain.MatchURI), по
// названию. Один запрос к серверу — для подсказок автозаполнения.
func (v *Vault) MatchLogins(ctx context.Context, webDomain, packageName string) ([]LoginMatch, error) {
	var result []LoginMatch
	err := v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		infos, err := svc.ListSecrets(ctx)
		if err != nil {
			return err
		}
		for _, info := range infos {
			if info.Type != domain.SecretTypeCredentials {
				continue
			}
			var c domain.CredentialsSecret
			if err := json.Unmarshal(info.Data, &c); err != nil {
				return fmt.Errorf("vault: decode credentials %q: %w", info.Name, err)
			}
			if slices.ContainsFunc(c.URIs, func(uri string) bool { return domain.MatchURI(uri, webDomain, packageName) }) {
				result = append(result, LoginMatch{Name: info.Name, Login: c.Login, Password: c.Password})
			}
		}
		return nil
	})
	slices.SortFunc(result, func(a, b LoginMatch) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return result, err
}

// GetSecret возвращает секрет с содержимым.
func (v *Vault) GetSecret(ctx context.Context, name string, secretType domain.SecretType) (*domain.SecretInfo, error) {
	var result *domain.SecretInfo
	err := v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		var err error
		result, err = svc.GetSecret(ctx, name, secretType)
		return err
	})
	return result, err
}

// CreateSecret создаёт секрет.
func (v *Vault) CreateSecret(ctx context.Context, payload *domain.SecretPayload) error {
	return v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		return svc.CreateSecret(ctx, payload)
	})
}

// UpdateSecret изменяет секрет.
func (v *Vault) UpdateSecret(ctx context.Context, payload *domain.SecretPayload) error {
	return v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		return svc.UpdateSecret(ctx, payload)
	})
}

// DeleteSecret удаляет секрет.
func (v *Vault) DeleteSecret(ctx context.Context, name string, secretType domain.SecretType) error {
	return v.withSecrets(ctx, v.cfg.RequestTimeout, func(ctx context.Context, svc service.SecretsService) error {
		return svc.DeleteSecret(ctx, name, secretType)
	})
}

// ChangePassword меняет пароль разблокированной учётки с перешифровкой всех секретов
// и оставляет хранилище разблокированным новым паролем. Остальные устройства
// после смены должны войти заново.
func (v *Vault) ChangePassword(ctx context.Context, oldPassword, newPassword string, kdf domain.KDFParams) error {
	err := v.withSecrets(ctx, bulkTimeout, func(ctx context.Context, svc service.SecretsService) error {
		reencrypt := func(newMasterKey []byte) ([]domain.ReencryptedSecret, error) {
			return svc.Reencrypt(ctx, newMasterKey)
		}
		err := v.auth.ChangePassword(ctx, v.login, oldPassword, newPassword, kdf, reencrypt)
		if errors.Is(err, domain.ErrInvalidCredentials) {
			// Здесь это неверный текущий пароль, а не отозванный токен.
			return ErrWrongPassword
		}
		return err
	})
	if err != nil {
		return err
	}
	return v.Unlock(ctx, newPassword)
}

// Logout отзывает токен на сервере (allSessions — на всех устройствах), удаляет
// сессию и блокирует хранилище.
func (v *Vault) Logout(ctx context.Context, allSessions bool) error {
	ctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()

	v.mu.Lock()
	err := v.auth.Logout(ctx, allSessions)
	if err != nil {
		v.mu.Unlock()
		return err
	}
	locked := v.lockLocked()
	v.login = ""
	v.mu.Unlock()
	if locked {
		v.notifyLock(LockSignedOut)
	}
	return nil
}

// DeleteAccount безвозвратно удаляет учётку со всеми секретами после проверки пароля.
func (v *Vault) DeleteAccount(ctx context.Context, password string) error {
	ctx, cancel := context.WithTimeout(ctx, v.cfg.RequestTimeout)
	defer cancel()

	v.mu.Lock()
	if v.login == "" {
		v.mu.Unlock()
		return ErrNotSignedIn
	}
	err := v.auth.DeleteAccount(ctx, v.login, password)
	if err != nil {
		v.mu.Unlock()
		return wrapCredentials(err, ErrWrongPassword)
	}
	locked := v.lockLocked()
	v.login = ""
	v.mu.Unlock()
	if locked {
		v.notifyLock(LockSignedOut)
	}
	return nil
}

// withSecrets выполняет операцию над разблокированным хранилищем.
//
// Если сервер не принял токен, хранилище блокируется и возвращается ErrSessionExpired.
// После успешной операции токен обновляется, если скоро истекает.
func (v *Vault) withSecrets(
	ctx context.Context, timeout time.Duration, op func(context.Context, service.SecretsService) error,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	v.mu.Lock()
	if v.secrets == nil {
		v.mu.Unlock()
		return ErrLocked
	}
	v.armIdleTimerLocked()

	err := op(ctx, v.secrets)
	if errors.Is(err, domain.ErrInvalidCredentials) {
		locked := v.lockLocked()
		v.mu.Unlock()
		if locked {
			v.notifyLock(LockSessionExpired)
		}
		return ErrSessionExpired
	}
	if err == nil {
		v.refreshTokenLocked(ctx)
	}
	v.mu.Unlock()
	return err
}

// unlockLocked переводит хранилище в разблокированное состояние. Вызывается под v.mu.
func (v *Vault) unlockLocked(login string, masterKey []byte) error {
	svc, err := service.NewSecretsService(v.client, masterKey)
	if err != nil {
		clear(masterKey)
		return fmt.Errorf("vault: %w", err)
	}
	v.login = login
	v.masterKey = masterKey
	v.secrets = svc
	v.armIdleTimerLocked()
	return nil
}

// lockLocked стирает ключи. Возвращает true, если хранилище было разблокировано.
// Вызывается под v.mu; OnLock вызывающий вызывает сам, после освобождения v.mu.
func (v *Vault) lockLocked() bool {
	v.idleGen++
	if v.idleTimer != nil {
		v.idleTimer.Stop()
		v.idleTimer = nil
	}
	if v.secrets == nil {
		return false
	}
	v.secrets.Wipe()
	v.secrets = nil
	clear(v.masterKey)
	v.masterKey = nil
	return true
}

// armIdleTimerLocked перезапускает таймер автоблокировки. Вызывается под v.mu.
// Поколение idleGen защищает от срабатывания таймера, который был перезапущен
// или остановлен, пока его функция ждала v.mu.
func (v *Vault) armIdleTimerLocked() {
	if v.cfg.AutoLockAfter <= 0 || v.secrets == nil {
		return
	}
	v.idleGen++
	gen := v.idleGen
	if v.idleTimer != nil {
		v.idleTimer.Stop()
	}
	v.idleTimer = time.AfterFunc(v.cfg.AutoLockAfter, func() {
		v.mu.Lock()
		locked := v.idleGen == gen && v.lockLocked()
		v.mu.Unlock()
		if locked {
			v.notifyLock(LockIdle)
		}
	})
}

// refreshTokenLocked обновляет токен, если он скоро истечёт. Вызывается под v.mu
// только после успешного запроса: так отозванный токен не продлевается.
// Ошибка обновления не мешает операции: если токен действительно перестанет
// действовать, следующий запрос заблокирует хранилище.
func (v *Vault) refreshTokenLocked(ctx context.Context) {
	expiresAt, ok := tokenExpiry(v.tokens.Token())
	if ok && time.Until(expiresAt) > v.cfg.RefreshMargin {
		return
	}
	_ = v.auth.RefreshToken(ctx, v.login, v.masterKey)
}

// notifyLock вызывает OnLock, если он задан.
func (v *Vault) notifyLock(reason LockReason) {
	if v.cfg.OnLock != nil {
		v.cfg.OnLock(reason)
	}
}

// tokenExpiry читает срок действия токена без проверки подписи: клиенту нужна
// только дата, подлинность проверяет сервер.
func tokenExpiry(token string) (time.Time, bool) {
	if token == "" {
		return time.Time{}, false
	}
	claims := jwt.RegisteredClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil || claims.ExpiresAt == nil {
		return time.Time{}, false
	}
	return claims.ExpiresAt.Time, true
}

// wrapCredentials заменяет domain.ErrInvalidCredentials на target, сохраняя
// остальные ошибки как есть.
func wrapCredentials(err, target error) error {
	if errors.Is(err, domain.ErrInvalidCredentials) {
		return target
	}
	return err
}
