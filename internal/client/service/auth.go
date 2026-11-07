// Package service реализует бизнес-логику клиента Shron.
// Обеспечивает аутентификацию пользователей, управление секретами,
// а также шифрование и расшифровку данных на стороне клиента.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/client/kdfpin"
	"github.com/F3dosik/Shron/internal/client/session"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/F3dosik/Shron/pkg/crypto"
)

// AuthService определяет интерфейс для аутентификации пользователя.
type AuthService interface {
	// CreateUser регистрирует нового пользователя на сервере.
	// Генерирует соль, деривирует мастер-ключ через Argon2id и отправляет на сервер.
	// kdf — параметры Argon2id новой учётки (обычно domain.DefaultKDFParams).
	CreateUser(ctx context.Context, login, password string, kdf domain.KDFParams) error

	// CreateTemporaryUser регистрирует пользователя со сгенерированным временным паролем
	// (доступно только на административном порту сервера). Возвращает пароль и время,
	// до которого им можно войти; при первом входе пароль нужно сменить.
	CreateTemporaryUser(ctx context.Context, login string, kdf domain.KDFParams) (password string, expiresAt time.Time, err error)

	// Login аутентифицирует пользователя и сохраняет сессию (логин + JWT токен) в файл.
	// Запрашивает соль с сервера, деривирует ключ аутентификации и получает токен.
	// Для временного пароля возвращает passwordChangeRequired == true: сессия не
	// сохраняется, а полученный токен годится только для ChangePassword.
	Login(ctx context.Context, login, password string) (passwordChangeRequired bool, err error)

	// Unlock проверяет мастер-пароль на сервере и возвращает masterKey для деривации
	// ключей шифрования. Используется перед операциями с секретами: без проверки неверный
	// пароль дал бы другие ключи, и секрет был бы сохранён в недоступном для пользователя виде.
	// Возвращает domain.ErrInvalidCredentials, если пароль неверный.
	Unlock(ctx context.Context, login, password string) ([]byte, error)

	// Logout отзывает токен на сервере и удаляет локальную сессию.
	// allSessions == true отзывает все токены пользователя (выход на всех устройствах).
	Logout(ctx context.Context, allSessions bool) error

	// ChangePassword меняет пароль: генерирует новую соль, выводит ключи из нового пароля,
	// получает от reencrypt перешифрованные новым мастер-ключом секреты и отправляет всё
	// на сервер. Сохраняет новый токен. reencrypt может быть nil, если секретов нет.
	// newKDF — параметры Argon2id для нового пароля.
	ChangePassword(ctx context.Context, login, oldPassword, newPassword string, newKDF domain.KDFParams, reencrypt Reencryptor) error

	// DeleteAccount безвозвратно удаляет учётку login вместе со всеми секретами,
	// подтверждая пароль на сервере, и удаляет локальную сессию.
	// Возвращает domain.ErrInvalidCredentials при неверном пароле.
	DeleteAccount(ctx context.Context, login, password string) error

	// RefreshToken получает новый токен по уже выведенному мастер-ключу, без пароля
	// и без Argon2id, и сохраняет его в сессию. Нужен клиентам, которые держат хранилище
	// разблокированным дольше срока жизни токена.
	RefreshToken(ctx context.Context, login string, masterKey []byte) error

	// ForgetKDFParams забывает параметры Argon2id учётки, запомненные на этом устройстве.
	// Нужен, когда вход отклонён с domain.KDFDowngradeError, а пользователь подтвердил,
	// что сам сменил пароль с более слабыми параметрами на другом устройстве.
	ForgetKDFParams(login string) error
}

// Reencryptor перешифровывает все секреты пользователя ключами от newMasterKey.
type Reencryptor func(newMasterKey []byte) ([]domain.ReencryptedSecret, error)

// authService реализует AuthService.
type authService struct {
	// client используется для взаимодействия с gRPC сервером аутентификации.
	client grpcclient.AuthClient
	// sessionPath — путь к файлу для хранения сессии пользователя (логин + JWT токен).
	sessionPath string
	// tokens — токен, с которым соединение выполняет запросы; обновляется после входа.
	tokens *grpcclient.TokenStore
	// kdfPins — параметры Argon2id, запомненные на устройстве (см. kdfpin), и адрес
	// сервера, к которому они относятся. nil — параметры не проверяются.
	kdfPins *kdfpin.Store
	server  string
}

// AuthOption настраивает authService.
type AuthOption func(*authService)

// WithKDFPins включает защиту от подмены параметров Argon2id: параметры учёток на сервере
// server запоминаются в pins, а более слабые параметры от сервера отклоняются.
func WithKDFPins(pins *kdfpin.Store, server string) AuthOption {
	return func(s *authService) {
		s.kdfPins, s.server = pins, server
	}
}

// NewAuthService создаёт новый authService с заданным gRPC клиентом, путём к файлу сессии
// и хранилищем токена соединения (может быть nil, если обновлять токен в соединении не нужно).
func NewAuthService(
	client grpcclient.AuthClient, sessionPath string, tokens *grpcclient.TokenStore, opts ...AuthOption,
) AuthService {
	s := &authService{client: client, sessionPath: sessionPath, tokens: tokens}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateUser регистрирует нового пользователя.
// Генерирует случайную соль, деривирует мастер-ключ через Argon2id и отправляет на сервер
// производный от него ключ аутентификации (сам masterKey клиент не покидает).
func (s *authService) CreateUser(ctx context.Context, login, password string, kdf domain.KDFParams) error {
	if _, err := s.register(ctx, login, password, kdf, false); err != nil {
		return fmt.Errorf("authService.CreateUser: %w", err)
	}
	if err := s.kdfPins.Remember(s.server, login, kdf); err != nil {
		return fmt.Errorf("authService.CreateUser: %w", err)
	}
	return nil
}

// CreateTemporaryUser регистрирует пользователя со случайным временным паролем.
func (s *authService) CreateTemporaryUser(ctx context.Context, login string, kdf domain.KDFParams) (string, time.Time, error) {
	password, err := generateTemporaryPassword()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("authService.CreateTemporaryUser: %w", err)
	}
	expiresAt, err := s.register(ctx, login, password, kdf, true)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("authService.CreateTemporaryUser: %w", err)
	}
	if expiresAt == nil {
		return "", time.Time{}, errors.New("authService.CreateTemporaryUser: server did not mark the password as temporary")
	}
	return password, *expiresAt, nil
}

// register выводит ключи из пароля с новой солью и регистрирует пользователя.
// Параметры проверяются до деривации, чтобы ошибка появилась сразу, а не после Argon2id.
func (s *authService) register(
	ctx context.Context, login, password string, kdf domain.KDFParams, temporary bool,
) (*time.Time, error) {
	if err := kdf.Validate(); err != nil {
		return nil, err
	}
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return nil, err
	}

	_, authKey, err := deriveKeys(password, salt, kdf)
	if err != nil {
		return nil, err
	}
	return s.client.CreateUser(ctx, domain.Credentials{Login: login, AuthKey: authKey}, salt, kdf, temporary)
}

// temporaryPasswordAlphabet — символы временного пароля без похожих (0/O, 1/l/I).
const temporaryPasswordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generateTemporaryPassword возвращает пароль вида xxxx-xxxx-xxxx-xxxx
// (16 случайных символов из 55, около 92 бит энтропии).
func generateTemporaryPassword() (string, error) {
	const groups, groupLen = 4, 4
	var b strings.Builder
	for g := range groups {
		if g > 0 {
			b.WriteByte('-')
		}
		for range groupLen {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(temporaryPasswordAlphabet))))
			if err != nil {
				return "", err
			}
			b.WriteByte(temporaryPasswordAlphabet[n.Int64()])
		}
	}
	return b.String(), nil
}

// Login аутентифицирует пользователя на сервере.
// Запрашивает соль по логину, деривирует ключ аутентификации, получает JWT токен
// и сохраняет сессию (логин + токен) в файл для последующих вызовов.
func (s *authService) Login(ctx context.Context, login, password string) (bool, error) {
	_, authKey, kdf, err := s.userKeys(ctx, login, password)
	if err != nil {
		return false, fmt.Errorf("authService.Login: %w", err)
	}
	token, changeRequired, err := s.client.Login(ctx, domain.Credentials{Login: login, AuthKey: authKey})
	if err != nil {
		return false, fmt.Errorf("authService.Login: %w", err)
	}
	if err := s.kdfPins.Remember(s.server, login, kdf); err != nil {
		return false, fmt.Errorf("authService.Login: %w", err)
	}

	if changeRequired {
		// Токен нужен только для смены пароля в этом же процессе; в файл сессии он
		// не попадает, чтобы прерванная смена не оставила полувход.
		s.tokens.SetToken(token)
		return true, nil
	}

	if err := s.saveSession(login, token); err != nil {
		return false, fmt.Errorf("authService.Login: %w", err)
	}

	return false, nil
}

// Unlock получает соль пользователя, деривирует masterKey через Argon2id и проверяет
// пароль, выполняя Login с производным ключом аутентификации. Полученный новый токен
// сохраняется в сессию и используется соединением, поэтому короткий срок жизни токена
// не требует частого повторного входа.
func (s *authService) Unlock(ctx context.Context, login, password string) ([]byte, error) {
	masterKey, authKey, kdf, err := s.userKeys(ctx, login, password)
	if err != nil {
		return nil, fmt.Errorf("authService.Unlock: %w", err)
	}
	token, changeRequired, err := s.client.Login(ctx, domain.Credentials{Login: login, AuthKey: authKey})
	if err != nil {
		return nil, fmt.Errorf("authService.Unlock: %w", err)
	}
	if changeRequired {
		return nil, fmt.Errorf("authService.Unlock: %w", domain.ErrPasswordChangeRequired)
	}
	if err := s.kdfPins.Remember(s.server, login, kdf); err != nil {
		return nil, fmt.Errorf("authService.Unlock: %w", err)
	}
	if err := s.saveSession(login, token); err != nil {
		return nil, fmt.Errorf("authService.Unlock: %w", err)
	}

	return masterKey, nil
}

// userKeys запрашивает соль и параметры Argon2id пользователя и выводит из пароля
// мастер-ключ и ключ аутентификации. Параметры от сервера проверяются: подменённый
// сервер мог бы прислать слабые параметры, чтобы удешевить перебор пароля по authKey,
// или огромные, чтобы клиент завис. Кроме общих границ, параметры не должны быть слабее
// запомненных на этом устройстве (kdfpin). Возвращает и сами параметры, чтобы после
// успешного входа их запомнить.
func (s *authService) userKeys(
	ctx context.Context, login, password string,
) (masterKey, authKey []byte, kdf domain.KDFParams, err error) {
	salt, kdf, err := s.client.GetSalt(ctx, login)
	if err != nil {
		return nil, nil, kdf, err
	}
	if err := kdf.Validate(); err != nil {
		return nil, nil, kdf, fmt.Errorf("server sent unacceptable kdf params: %w", err)
	}
	known, ok, err := s.kdfPins.Get(s.server, login)
	if err != nil {
		return nil, nil, kdf, err
	}
	if ok && kdf.WeakerThan(known) {
		return nil, nil, kdf, &domain.KDFDowngradeError{Known: known, Received: kdf}
	}
	masterKey, authKey, err = deriveKeys(password, salt, kdf)
	return masterKey, authKey, kdf, err
}

// deriveKeys выводит мастер-ключ Argon2id(password, salt) и ключ аутентификации
// HKDF(masterKey, "auth"). Ключ аутентификации отправляется на сервер и независим от
// ключей шифрования, которые выводятся из того же masterKey с другим info.
func deriveKeys(password string, salt []byte, kdf domain.KDFParams) (masterKey, authKey []byte, err error) {
	masterKey = crypto.DeriveKey(password, salt, kdf)
	authKey, err = crypto.HKDF(masterKey, crypto.InfoAuth)
	if err != nil {
		return nil, nil, err
	}
	return masterKey, authKey, nil
}

// Logout отзывает токен на сервере, затем удаляет файл сессии.
// Если отзывается только текущий токен, а сервер его уже не принимает (истёк или отозван),
// локальная сессия всё равно удаляется: такой токен и так бесполезен.
func (s *authService) Logout(ctx context.Context, allSessions bool) error {
	err := s.client.Logout(ctx, allSessions)
	if err != nil && (allSessions || !errors.Is(err, domain.ErrInvalidCredentials)) {
		return fmt.Errorf("authService.Logout: %w", err)
	}

	if err := os.Remove(s.sessionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("authService.Logout: %w", err)
	}
	s.tokens.SetToken("")
	return nil
}

// saveSession сохраняет сессию в файл и передаёт токен соединению.
func (s *authService) saveSession(login, token string) error {
	if err := session.Save(s.sessionPath, &session.Session{Login: login, Token: token}); err != nil {
		return err
	}
	s.tokens.SetToken(token)
	return nil
}

// ChangePassword меняет пароль и перешифровывает секреты.
func (s *authService) ChangePassword(
	ctx context.Context, login, oldPassword, newPassword string, newKDF domain.KDFParams, reencrypt Reencryptor,
) error {
	if err := newKDF.Validate(); err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}
	_, oldAuthKey, _, err := s.userKeys(ctx, login, oldPassword)
	if err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}

	newSalt, err := crypto.GenerateSalt()
	if err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}
	newMasterKey, newAuthKey, err := deriveKeys(newPassword, newSalt, newKDF)
	if err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}

	var secrets []domain.ReencryptedSecret
	if reencrypt != nil {
		if secrets, err = reencrypt(newMasterKey); err != nil {
			return fmt.Errorf("authService.ChangePassword: %w", err)
		}
	}

	token, err := s.client.ChangePassword(ctx, domain.PasswordChange{
		OldAuthKey: oldAuthKey,
		NewSalt:    newSalt,
		NewKDF:     newKDF,
		NewAuthKey: newAuthKey,
	}, secrets)
	if err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}

	// Новые параметры выбраны на этом устройстве, поэтому запоминаются как есть, даже
	// если они слабее прежних.
	if err := s.kdfPins.Remember(s.server, login, newKDF); err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}
	if err := s.saveSession(login, token); err != nil {
		return fmt.Errorf("authService.ChangePassword: %w", err)
	}
	return nil
}

// DeleteAccount удаляет учётку пользователя и локальную сессию.
func (s *authService) DeleteAccount(ctx context.Context, login, password string) error {
	_, authKey, _, err := s.userKeys(ctx, login, password)
	if err != nil {
		return fmt.Errorf("authService.DeleteAccount: %w", err)
	}
	if err := s.client.DeleteAccount(ctx, authKey); err != nil {
		return fmt.Errorf("authService.DeleteAccount: %w", err)
	}
	// Логин освободился: новая учётка с тем же именем может иметь любые параметры.
	if err := s.kdfPins.Forget(s.server, login); err != nil {
		return fmt.Errorf("authService.DeleteAccount: аккаунт удалён: %w", err)
	}

	if err := os.Remove(s.sessionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("authService.DeleteAccount: аккаунт удалён, но не удалось удалить сеанс: %w", err)
	}
	s.tokens.SetToken("")
	return nil
}

// RefreshToken получает новый токен по мастер-ключу и сохраняет сессию.
func (s *authService) RefreshToken(ctx context.Context, login string, masterKey []byte) error {
	authKey, err := crypto.HKDF(masterKey, crypto.InfoAuth)
	if err != nil {
		return fmt.Errorf("authService.RefreshToken: %w", err)
	}
	token, changeRequired, err := s.client.Login(ctx, domain.Credentials{Login: login, AuthKey: authKey})
	if err != nil {
		return fmt.Errorf("authService.RefreshToken: %w", err)
	}
	if changeRequired {
		return fmt.Errorf("authService.RefreshToken: %w", domain.ErrPasswordChangeRequired)
	}
	if err := s.saveSession(login, token); err != nil {
		return fmt.Errorf("authService.RefreshToken: %w", err)
	}
	return nil
}

// ForgetKDFParams забывает запомненные параметры Argon2id учётки.
func (s *authService) ForgetKDFParams(login string) error {
	if err := s.kdfPins.Forget(s.server, login); err != nil {
		return fmt.Errorf("authService.ForgetKDFParams: %w", err)
	}
	return nil
}
