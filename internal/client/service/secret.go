package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/F3dosik/Shron/internal/client/grpcclient"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/F3dosik/Shron/pkg/crypto"
)

// ErrIntegrity возвращается, если расшифрованный секрет не соответствует записи, под которой
// он хранится на сервере: сервер (или тот, кто его контролирует) подменил данные одного
// секрета данными другого.
var ErrIntegrity = errors.New("нарушена целостность: данные записи не соответствуют её имени и типу")

// SecretsService определяет интерфейс для работы с секретами пользователя.
type SecretsService interface {
	// ListSecrets возвращает список всех секретов пользователя.
	// Расшифровывает данные каждого секрета перед возвратом.
	ListSecrets(ctx context.Context) ([]*domain.SecretInfo, error)

	// CreateSecret создаёт новый секрет на сервере.
	// Шифрует payload и вычисляет blind index перед отправкой.
	CreateSecret(ctx context.Context, payload *domain.SecretPayload) error

	// UpdateSecret обновляет существующий секрет на сервере.
	// Шифрует payload и вычисляет blind index перед отправкой.
	UpdateSecret(ctx context.Context, payload *domain.SecretPayload) error

	// GetSecret возвращает секрет по имени и типу.
	// Расшифровывает данные перед возвратом.
	GetSecret(ctx context.Context, name string, secretType domain.SecretType) (*domain.SecretInfo, error)

	// DeleteSecret удаляет секрет по имени и типу.
	DeleteSecret(ctx context.Context, name string, secretType domain.SecretType) error

	// Reencrypt загружает все секреты, расшифровывает их текущим ключом и шифрует
	// ключами, выведенными из newMasterKey, вместе с новыми blind index.
	// Используется при смене пароля.
	Reencrypt(ctx context.Context, newMasterKey []byte) ([]domain.ReencryptedSecret, error)

	// BlindIndex вычисляет blind index секрета с именем name и типом secretType.
	BlindIndex(name string, secretType domain.SecretType) string

	// Seal шифрует payload так же, как CreateSecret, и возвращает blind index и данные,
	// не отправляя их на сервер (например, для очереди изменений без сети).
	Seal(payload *domain.SecretPayload) (blindIndex string, data []byte, err error)

	// Open расшифровывает данные записи с blind index и проверяет их целостность,
	// как GetSecret, без обращения к серверу.
	Open(blindIndex string, data []byte) (*domain.SecretPayload, error)

	// Wipe обнуляет ключ blind index и отпускает шифр. После вызова сервис непригоден.
	// Ключ AES внутри шифра стандартной библиотеки обнулить нельзя: он станет мусором
	// и будет собран сборщиком, поэтому Wipe уменьшает, но не исключает время жизни
	// ключей в памяти.
	Wipe()
}

// secretsService реализует SecretsService.
type secretsService struct {
	// client используется для взаимодействия с gRPC сервером секретов.
	client grpcclient.SecretsClient
	// hmacKey используется для вычисления blind index через HMAC-SHA256.
	hmacKey []byte
	// cipher используется для шифрования и расшифровки данных секретов.
	cipher crypto.Cipher
}

// NewSecretsService создаёт новый secretsService.
// Деривирует ключ шифрования и ключ HMAC из masterKey через HKDF.
func NewSecretsService(client grpcclient.SecretsClient, masterKey []byte) (SecretsService, error) {
	return newSecretsService(client, masterKey)
}

// newSecretsService создаёт *secretsService; нужен там, где требуются ключи
// конкретного мастер-ключа, например при перешифровке.
func newSecretsService(client grpcclient.SecretsClient, masterKey []byte) (*secretsService, error) {
	hmacKey, err := crypto.HKDF(masterKey, crypto.InfoBlindIndex)
	if err != nil {
		return nil, fmt.Errorf("new secrets service: %w", err)
	}

	encKey, err := crypto.HKDF(masterKey, crypto.InfoEncryption)
	if err != nil {
		return nil, fmt.Errorf("new secrets service: %w", err)
	}
	cipher, err := crypto.NewAESCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("newSecretsService: %w", err)
	}
	return &secretsService{client: client, hmacKey: hmacKey, cipher: cipher}, nil
}

// encryptPayload сериализует payload в JSON и шифрует его через AES-256-GCM.
func (s *secretsService) encryptPayload(payload *domain.SecretPayload) ([]byte, error) {
	plaintext, err := padPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	ciphertext, err := s.cipher.Encrypt(plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypt payload: %w", err)
	}
	return ciphertext, nil
}

// paddingBlock — размер блока, до которого дополняется открытый текст секрета.
const paddingBlock = 256

// paddedPayload — формат открытого текста секрета: payload и поле-заполнитель.
// Поле pad при расшифровке игнорируется, в том числе клиентами без этого поля.
type paddedPayload struct {
	*domain.SecretPayload
	Pad string `json:"pad"`
}

// padPayload сериализует payload в JSON и дополняет его пробелами в поле pad так,
// чтобы длина была кратна paddingBlock. AES-GCM сохраняет длину открытого текста,
// и без дополнения сервер по размеру шифротекста оценил бы длину пароля или заметки.
func padPayload(payload *domain.SecretPayload) ([]byte, error) {
	base, err := json.Marshal(paddedPayload{SecretPayload: payload})
	if err != nil {
		return nil, err
	}
	// Пробелы кодируются в JSON один к одному, поэтому длину можно рассчитать заранее.
	padLen := (paddingBlock - len(base)%paddingBlock) % paddingBlock
	if padLen == 0 {
		return base, nil
	}
	return json.Marshal(paddedPayload{SecretPayload: payload, Pad: strings.Repeat(" ", padLen)})
}

// decryptPayload расшифровывает данные, десериализует их в SecretPayload и проверяет,
// что имя и тип внутри шифротекста дают тот же blind index, под которым запись хранится.
// AES-GCM гарантирует, что шифротекст создан владельцем ключа, а эта проверка —
// что сервер вернул именно запрошенный секрет, а не другой секрет того же пользователя.
func (s *secretsService) decryptPayload(blindIndex string, data []byte) (*domain.SecretPayload, error) {
	plaintext, err := s.cipher.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}

	var payload domain.SecretPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	expected := crypto.BlindIndex(payload.Name, payload.Type, s.hmacKey)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(blindIndex)) != 1 {
		return nil, ErrIntegrity
	}
	return &payload, nil
}

// ListSecrets возвращает список всех секретов пользователя.
// Расшифровывает данные каждого секрета и заполняет timestamps с сервера.
func (s *secretsService) ListSecrets(ctx context.Context) ([]*domain.SecretInfo, error) {
	secrets, err := s.client.ListSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("secretsService.ListSecrets: %w", err)
	}
	secretInformations := make([]*domain.SecretInfo, 0, len(secrets))
	for _, secret := range secrets {
		payload, err := s.decryptPayload(secret.BlindIndex, secret.Data)
		if err != nil {
			return nil, fmt.Errorf("secretService.ListSecrets: %w", err)
		}
		secretInformations = append(secretInformations, &domain.SecretInfo{
			SecretPayload: *payload,
			CreatedAt:     secret.CreatedAt,
			UpdatedAt:     secret.UpdatedAt,
		})
	}
	return secretInformations, nil
}

// CreateSecret создаёт новый секрет на сервере.
// Вычисляет blind index, шифрует payload и отправляет на сервер.
func (s *secretsService) CreateSecret(
	ctx context.Context, payload *domain.SecretPayload,
) error {
	blindIndex := crypto.BlindIndex(payload.Name, payload.Type, s.hmacKey)

	ciphertext, err := s.encryptPayload(payload)
	if err != nil {
		return fmt.Errorf("secretService.CreateSecret: %w", err)
	}

	if err := s.client.CreateSecret(ctx, blindIndex, ciphertext); err != nil {
		return fmt.Errorf("secretService.CreateSecret: %w", err)
	}
	return nil
}

// UpdateSecret обновляет существующий секрет на сервере.
// Вычисляет blind index, шифрует payload и отправляет на сервер.
func (s *secretsService) UpdateSecret(ctx context.Context, payload *domain.SecretPayload) error {
	blindIndex := crypto.BlindIndex(payload.Name, payload.Type, s.hmacKey)

	ciphertext, err := s.encryptPayload(payload)
	if err != nil {
		return fmt.Errorf("secretService.UpdateSecret: %w", err)
	}

	if err := s.client.UpdateSecret(ctx, blindIndex, ciphertext); err != nil {
		return fmt.Errorf("secretService.UpdateSecret: %w", err)
	}
	return nil
}

// GetSecret возвращает секрет по имени и типу.
// Вычисляет blind index, получает зашифрованные данные и расшифровывает их.
func (s *secretsService) GetSecret(ctx context.Context, name string, secretType domain.SecretType) (*domain.SecretInfo, error) {
	blindIndex := crypto.BlindIndex(name, secretType, s.hmacKey)
	secret, err := s.client.GetSecret(ctx, blindIndex)
	if err != nil {
		return nil, fmt.Errorf("secretService.GetSecret: %w", err)
	}

	payload, err := s.decryptPayload(blindIndex, secret.Data)
	if err != nil {
		return nil, fmt.Errorf("secretService.GetSecret: %w", err)
	}

	return &domain.SecretInfo{
		SecretPayload: *payload,
		CreatedAt:     secret.CreatedAt,
		UpdatedAt:     secret.UpdatedAt,
	}, nil
}

// DeleteSecret удаляет секрет по имени и типу.
// Вычисляет blind index и отправляет запрос на удаление.
func (s *secretsService) DeleteSecret(ctx context.Context, name string, secretType domain.SecretType) error {
	blindIndex := crypto.BlindIndex(name, secretType, s.hmacKey)
	if err := s.client.DeleteSecret(ctx, blindIndex); err != nil {
		return fmt.Errorf("secretService.DeleteSecret: %w", err)
	}
	return nil
}

// Reencrypt перешифровывает все секреты ключами от нового мастер-ключа.
// Каждый секрет проходит проверку целостности, поэтому подменённые сервером данные
// не будут перешифрованы под новым паролем.
func (s *secretsService) Reencrypt(ctx context.Context, newMasterKey []byte) ([]domain.ReencryptedSecret, error) {
	target, err := newSecretsService(nil, newMasterKey)
	if err != nil {
		return nil, fmt.Errorf("secretsService.Reencrypt: %w", err)
	}

	secrets, err := s.client.ListSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("secretsService.Reencrypt: %w", err)
	}

	result := make([]domain.ReencryptedSecret, 0, len(secrets))
	for _, secret := range secrets {
		payload, err := s.decryptPayload(secret.BlindIndex, secret.Data)
		if err != nil {
			return nil, fmt.Errorf("secretsService.Reencrypt: %w", err)
		}
		data, err := target.encryptPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("secretsService.Reencrypt: %w", err)
		}
		result = append(result, domain.ReencryptedSecret{
			OldBlindIndex:     secret.BlindIndex,
			NewBlindIndex:     crypto.BlindIndex(payload.Name, payload.Type, target.hmacKey),
			Data:              data,
			ExpectedUpdatedAt: secret.UpdatedAt,
		})
	}
	return result, nil
}

// BlindIndex вычисляет blind index секрета.
func (s *secretsService) BlindIndex(name string, secretType domain.SecretType) string {
	return crypto.BlindIndex(name, secretType, s.hmacKey)
}

// Seal шифрует payload без отправки на сервер.
func (s *secretsService) Seal(payload *domain.SecretPayload) (string, []byte, error) {
	data, err := s.encryptPayload(payload)
	if err != nil {
		return "", nil, fmt.Errorf("secretsService.Seal: %w", err)
	}
	return s.BlindIndex(payload.Name, payload.Type), data, nil
}

// Open расшифровывает данные записи без обращения к серверу.
func (s *secretsService) Open(blindIndex string, data []byte) (*domain.SecretPayload, error) {
	payload, err := s.decryptPayload(blindIndex, data)
	if err != nil {
		return nil, fmt.Errorf("secretsService.Open: %w", err)
	}
	return payload, nil
}

// Wipe обнуляет ключи сервиса.
func (s *secretsService) Wipe() {
	clear(s.hmacKey)
	s.hmacKey = nil
	s.cipher = nil
}
