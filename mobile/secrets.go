package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/F3dosik/Shron/internal/domain"
)

// Типы секретов.
const (
	TypeCredentials = string(domain.SecretTypeCredentials)
	TypeText        = string(domain.SecretTypeText)
	TypeCard        = string(domain.SecretTypeCard)
	TypeBinary      = string(domain.SecretTypeBinary)
)

// maxFileSize — ограничение на размер файла для бинарного секрета. Сервер принимает
// до SECRET_MAX_SIZE зашифрованных данных (по умолчанию 1 MiB); файл хранится в base64
// внутри JSON, поэтому реальный предел меньше — около 750 КБ.
const maxFileSize = 32 << 20

// SecretSummary — строка списка секретов, без содержимого.
// Время — миллисекунды Unix (0 — неизвестно), в Kotlin: Instant.ofEpochMilli.
type SecretSummary struct {
	Name     string
	Type     string
	Metadata string
	// Group — группа секрета; пусто — без группы.
	Group     string
	CreatedAt int64
	UpdatedAt int64
}

// SecretList — список секретов: gomobile не передаёт срезы структур.
// В Kotlin (int в Go — Long): (0L until list.len()).map { list.get(it) }.
type SecretList struct {
	items []*SecretSummary
}

// Len возвращает число секретов.
func (l *SecretList) Len() int {
	return len(l.items)
}

// Get возвращает секрет с индексом i или nil, если индекс вне списка.
func (l *SecretList) Get(i int) *SecretSummary {
	if i < 0 || i >= len(l.items) {
		return nil
	}
	return l.items[i]
}

// Secret — секрет с содержимым. Заполнены поля, соответствующие типу.
type Secret struct {
	Name      string
	Type      string
	Metadata  string
	Group     string
	CreatedAt int64
	UpdatedAt int64

	// credentials
	Login    string
	Password string
	// URIs — сайты и приложения логина, по одному на строку (gomobile не передаёт
	// срезы строк). Приложения — с префиксом «androidapp://».
	URIs string
	// text
	Text string
	// card
	CardNumber string
	CardHolder string
	CardExpiry string
	CardCVV    string
	// binary: только размер, содержимое — через GetFileContent, чтобы не копировать
	// файл в Kotlin при каждом просмотре.
	FileSize int
}

// SecretInput — данные формы создания или изменения секрета. В Kotlin создаётся
// конструктором SecretInput() с последующим заполнением полей.
type SecretInput struct {
	Name     string
	Type     string
	Metadata string
	// Group — группа секрета; пусто — без группы.
	Group string

	Login    string
	Password string
	// URIs — сайты и приложения логина: по одному на строку, через запятую или пробел.
	URIs string
	Text string

	CardNumber string
	CardHolder string
	CardExpiry string
	CardCVV    string

	// FileData — содержимое бинарного секрета (на Android — прочитанное через
	// ContentResolver). При изменении пустое значение оставляет прежнее содержимое.
	FileData []byte
}

// ListSecrets возвращает список секретов без содержимого.
func (a *App) ListSecrets() (*SecretList, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	list, err := v.ListSecrets(context.Background())
	if err != nil {
		return nil, toError(err)
	}
	result := &SecretList{items: make([]*SecretSummary, 0, len(list))}
	for _, s := range list {
		result.items = append(result.items, &SecretSummary{
			Name:      s.Name,
			Type:      string(s.Type),
			Metadata:  s.Metadata,
			Group:     s.Group,
			CreatedAt: unixMilli(s.CreatedAt),
			UpdatedAt: unixMilli(s.UpdatedAt),
		})
	}
	return result, nil
}

// GetSecret возвращает секрет с содержимым.
func (a *App) GetSecret(name, secretType string) (*Secret, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	t, err := parseType(secretType)
	if err != nil {
		return nil, err
	}
	info, err := v.GetSecret(context.Background(), name, t)
	if err != nil {
		return nil, toError(err)
	}
	return toSecret(info)
}

// GetFileContent возвращает содержимое бинарного секрета, например для сохранения
// через Storage Access Framework.
func (a *App) GetFileContent(name string) ([]byte, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	info, err := v.GetSecret(context.Background(), name, domain.SecretTypeBinary)
	if err != nil {
		return nil, toError(err)
	}
	var b domain.BinarySecret
	if err := json.Unmarshal(info.Data, &b); err != nil {
		return nil, toError(err)
	}
	return b.Data, nil
}

// SaveSecret создаёт (update == false) или изменяет секрет.
func (a *App) SaveSecret(input *SecretInput, update bool) error {
	if input == nil {
		return validationError("Нет данных секрета")
	}
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	payload, err := a.buildPayload(input, update)
	if err != nil {
		return err
	}
	if update {
		return toError(v.UpdateSecret(context.Background(), payload))
	}
	return toError(v.CreateSecret(context.Background(), payload))
}

// DeleteSecret удаляет секрет.
func (a *App) DeleteSecret(name, secretType string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	t, err := parseType(secretType)
	if err != nil {
		return err
	}
	return toError(v.DeleteSecret(context.Background(), name, t))
}

// buildPayload проверяет форму и собирает SecretPayload.
func (a *App) buildPayload(input *SecretInput, update bool) (*domain.SecretPayload, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, validationError("Укажите название")
	}
	t, err := parseType(input.Type)
	if err != nil {
		return nil, err
	}
	group, err := domain.NormalizeGroup(input.Group)
	if err != nil {
		return nil, toError(err)
	}

	var data any
	switch t {
	case domain.SecretTypeCredentials:
		if input.Login == "" && input.Password == "" {
			return nil, validationError("Укажите логин или пароль")
		}
		data = domain.CredentialsSecret{Login: input.Login, Password: input.Password, URIs: domain.ParseURIs(input.URIs)}
	case domain.SecretTypeText:
		if input.Text == "" {
			return nil, validationError("Введите текст")
		}
		data = domain.TextSecret{Text: input.Text}
	case domain.SecretTypeCard:
		card := domain.CardSecret{
			Number: strings.TrimSpace(input.CardNumber),
			Holder: strings.TrimSpace(input.CardHolder),
			Expiry: strings.TrimSpace(input.CardExpiry),
			CVV:    strings.TrimSpace(input.CardCVV),
		}
		if err := card.Validate(); err != nil {
			return nil, toError(err)
		}
		data = card
	case domain.SecretTypeBinary:
		content, err := a.binaryContent(name, input.FileData, update)
		if err != nil {
			return nil, err
		}
		data = domain.BinarySecret{Data: content}
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, toError(err)
	}
	return &domain.SecretPayload{Name: name, Type: t, Data: raw, Metadata: input.Metadata, Group: group}, nil
}

// binaryContent проверяет размер нового содержимого или, при изменении без нового
// содержимого, берёт прежнее.
func (a *App) binaryContent(name string, content []byte, update bool) ([]byte, error) {
	if len(content) > maxFileSize {
		return nil, validationError("Файл слишком большой")
	}
	if len(content) > 0 {
		return content, nil
	}
	if !update {
		return nil, validationError("Выберите файл")
	}
	return a.GetFileContent(name)
}

// parseType проверяет тип секрета.
func parseType(secretType string) (domain.SecretType, error) {
	t, err := domain.ParseSecretType(secretType)
	if err != nil {
		return "", validationError("Неизвестный тип секрета")
	}
	return t, nil
}

// toSecret переводит расшифрованный секрет в формат приложения.
func toSecret(info *domain.SecretInfo) (*Secret, error) {
	s := &Secret{
		Name:      info.Name,
		Type:      string(info.Type),
		Metadata:  info.Metadata,
		Group:     info.Group,
		CreatedAt: unixMilli(info.CreatedAt),
		UpdatedAt: unixMilli(info.UpdatedAt),
	}

	var err error
	switch info.Type {
	case domain.SecretTypeCredentials:
		var c domain.CredentialsSecret
		err = json.Unmarshal(info.Data, &c)
		s.Login, s.Password, s.URIs = c.Login, c.Password, strings.Join(c.URIs, "\n")
	case domain.SecretTypeText:
		var t domain.TextSecret
		err = json.Unmarshal(info.Data, &t)
		s.Text = t.Text
	case domain.SecretTypeCard:
		var c domain.CardSecret
		err = json.Unmarshal(info.Data, &c)
		s.CardNumber, s.CardHolder, s.CardExpiry, s.CardCVV = c.Number, c.Holder, c.Expiry, c.CVV
	case domain.SecretTypeBinary:
		var b domain.BinarySecret
		err = json.Unmarshal(info.Data, &b)
		s.FileSize = len(b.Data)
	default:
		return nil, toError(fmt.Errorf("unknown secret type %q", info.Type))
	}
	if err != nil {
		return nil, toError(err)
	}
	return s, nil
}

// unixMilli переводит время в миллисекунды Unix; 0 для нулевого времени.
func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
