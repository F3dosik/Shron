package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/F3dosik/Shron/internal/domain"
)

// maxFileSize — ограничение на размер файла для бинарного секрета. Сервер принимает
// до SECRET_MAX_SIZE зашифрованных данных (по умолчанию 1 MiB); файл хранится в base64
// внутри JSON, поэтому реальный предел меньше — около 750 КБ.
const maxFileSize = 32 << 20

// SecretSummary — строка списка секретов, без содержимого.
type SecretSummary struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Metadata  string `json:"metadata"`
	Group     string `json:"group"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Secret — секрет с содержимым. Заполнены поля, соответствующие типу.
type Secret struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Metadata  string `json:"metadata"`
	Group     string `json:"group"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`

	// credentials
	Login    string `json:"login"`
	Password string `json:"password"`
	// URIs — сайты и приложения логина, по одному на строку.
	URIs string `json:"uris"`
	// text
	Text string `json:"text"`
	// card
	CardNumber string `json:"cardNumber"`
	CardHolder string `json:"cardHolder"`
	CardExpiry string `json:"cardExpiry"`
	CardCVV    string `json:"cardCvv"`
	// binary: само содержимое в интерфейс не передаётся, только размер; сохранить
	// файл можно через ExportFile.
	FileSize int `json:"fileSize"`
}

// SecretInput — данные формы создания или изменения секрета.
type SecretInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Metadata string `json:"metadata"`
	Group    string `json:"group"`

	Login    string `json:"login"`
	Password string `json:"password"`
	// URIs — сайты и приложения логина: по одному на строку, через запятую или пробел.
	URIs string `json:"uris"`
	Text string `json:"text"`

	CardNumber string `json:"cardNumber"`
	CardHolder string `json:"cardHolder"`
	CardExpiry string `json:"cardExpiry"`
	CardCVV    string `json:"cardCvv"`

	// FilePath — файл для бинарного секрета (из ChooseFile). При изменении пустой
	// путь оставляет прежнее содержимое.
	FilePath string `json:"filePath"`
}

// ChosenFile — файл, выбранный в диалоге.
type ChosenFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ListSecrets возвращает список секретов без содержимого.
func (a *App) ListSecrets() ([]SecretSummary, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	list, err := v.ListSecrets(a.context())
	if err != nil {
		return nil, toUIError(err)
	}
	result := make([]SecretSummary, 0, len(list))
	for _, s := range list {
		result = append(result, SecretSummary{
			Name:      s.Name,
			Type:      string(s.Type),
			Metadata:  s.Metadata,
			Group:     s.Group,
			CreatedAt: formatTime(s.CreatedAt),
			UpdatedAt: formatTime(s.UpdatedAt),
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
	t, err := domain.ParseSecretType(secretType)
	if err != nil {
		return nil, validationError("Неизвестный тип записи")
	}
	info, err := v.GetSecret(a.context(), name, t)
	if err != nil {
		return nil, toUIError(err)
	}
	return toSecret(info)
}

// SaveSecret создаёт (update == false) или изменяет секрет.
func (a *App) SaveSecret(input SecretInput, update bool) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	payload, err := a.buildPayload(input, update)
	if err != nil {
		return err
	}
	if update {
		return toUIError(v.UpdateSecret(a.context(), payload))
	}
	return toUIError(v.CreateSecret(a.context(), payload))
}

// DeleteSecret удаляет секрет.
func (a *App) DeleteSecret(name, secretType string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	t, err := domain.ParseSecretType(secretType)
	if err != nil {
		return validationError("Неизвестный тип записи")
	}
	return toUIError(v.DeleteSecret(a.context(), name, t))
}

// ChooseFile показывает диалог выбора файла для бинарного секрета.
// Пустой путь в результате — пользователь отменил выбор.
func (a *App) ChooseFile() (*ChosenFile, error) {
	path, err := a.opts.UI.OpenFile("Файл для сохранения в хранилище", nil)
	if err != nil || path == "" {
		return &ChosenFile{}, toUIError(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, toUIError(err)
	}
	return &ChosenFile{Path: path, Name: filepath.Base(path), Size: info.Size()}, nil
}

// ExportFile сохраняет содержимое бинарного секрета в файл, выбранный в диалоге.
// Возвращает путь; пустой — пользователь отменил сохранение.
func (a *App) ExportFile(name string) (string, error) {
	v, err := a.currentVault()
	if err != nil {
		return "", err
	}
	info, err := v.GetSecret(a.context(), name, domain.SecretTypeBinary)
	if err != nil {
		return "", toUIError(err)
	}
	var b domain.BinarySecret
	if err := json.Unmarshal(info.Data, &b); err != nil {
		return "", toUIError(err)
	}

	path, err := a.opts.UI.SaveFile("Сохранить файл", name)
	if err != nil || path == "" {
		return "", toUIError(err)
	}
	// 0600: содержимое секрета не должно быть доступно другим пользователям системы.
	if err := os.WriteFile(path, b.Data, 0o600); err != nil {
		return "", toUIError(err)
	}
	return path, nil
}

// buildPayload проверяет форму и собирает SecretPayload.
func (a *App) buildPayload(input SecretInput, update bool) (*domain.SecretPayload, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, validationError("Укажите название")
	}
	t, err := domain.ParseSecretType(input.Type)
	if err != nil {
		return nil, validationError("Неизвестный тип записи")
	}
	group, err := domain.NormalizeGroup(input.Group)
	if err != nil {
		return nil, toUIError(err)
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
			return nil, toUIError(err)
		}
		data = card
	case domain.SecretTypeBinary:
		content, err := a.binaryContent(name, input.FilePath, update)
		if err != nil {
			return nil, err
		}
		data = domain.BinarySecret{Data: content}
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, toUIError(err)
	}
	return &domain.SecretPayload{Name: name, Type: t, Data: raw, Metadata: input.Metadata, Group: group}, nil
}

// binaryContent читает выбранный файл или, при изменении без нового файла, берёт
// прежнее содержимое секрета.
func (a *App) binaryContent(name, path string, update bool) ([]byte, error) {
	if path == "" {
		if !update {
			return nil, validationError("Выберите файл")
		}
		v, err := a.currentVault()
		if err != nil {
			return nil, err
		}
		info, err := v.GetSecret(a.context(), name, domain.SecretTypeBinary)
		if err != nil {
			return nil, toUIError(err)
		}
		var b domain.BinarySecret
		if err := json.Unmarshal(info.Data, &b); err != nil {
			return nil, toUIError(err)
		}
		return b.Data, nil
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, toUIError(err)
	}
	if stat.Size() > maxFileSize {
		return nil, validationError("Файл слишком большой")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, toUIError(err)
	}
	return content, nil
}

// toSecret переводит расшифрованный секрет в формат интерфейса.
func toSecret(info *domain.SecretInfo) (*Secret, error) {
	s := &Secret{
		Name:      info.Name,
		Type:      string(info.Type),
		Metadata:  info.Metadata,
		Group:     info.Group,
		CreatedAt: formatTime(info.CreatedAt),
		UpdatedAt: formatTime(info.UpdatedAt),
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
		return nil, toUIError(fmt.Errorf("unknown secret type %q", info.Type))
	}
	if err != nil {
		return nil, toUIError(err)
	}
	return s, nil
}

// formatTime форматирует время для интерфейса (RFC 3339 в UTC; пустая строка для нуля).
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
