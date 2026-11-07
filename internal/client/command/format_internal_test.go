package command

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeInfo(t *testing.T, typ domain.SecretType, payload any, metadata string) *domain.SecretInfo {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return &domain.SecretInfo{
		SecretPayload: domain.SecretPayload{
			Name:     "test",
			Type:     typ,
			Data:     raw,
			Metadata: metadata,
		},
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestFormatPretty_Credentials(t *testing.T) {
	info := makeInfo(t, domain.SecretTypeCredentials,
		domain.CredentialsSecret{Login: "ivan", Password: "s3cret"}, "github")

	out, err := formatPretty(info)
	require.NoError(t, err)

	s := string(out)
	assert.Contains(t, s, "Название:    test")
	assert.Contains(t, s, "Комментарий: github")
	assert.NotContains(t, s, "Папка:", "empty group should be omitted")
	assert.Contains(t, s, "Логин:    ivan")
	assert.Contains(t, s, "Пароль:   s3cret")
}

func TestFormatPretty_Group(t *testing.T) {
	info := makeInfo(t, domain.SecretTypeText, domain.TextSecret{Text: "hello"}, "")
	info.Group = "Работа"

	out, err := formatPretty(info)
	require.NoError(t, err)
	assert.Contains(t, string(out), "Папка:       Работа")
}

func TestFormatPretty_Text(t *testing.T) {
	info := makeInfo(t, domain.SecretTypeText, domain.TextSecret{Text: "hello"}, "")

	out, err := formatPretty(info)
	require.NoError(t, err)

	s := string(out)
	assert.Contains(t, s, "hello")
	assert.NotContains(t, s, "Комментарий:", "empty metadata should be omitted")
}

func TestFormatPretty_Card(t *testing.T) {
	info := makeInfo(t, domain.SecretTypeCard, domain.CardSecret{
		Number: "4242424242424242",
		Holder: "IVAN IVANOV",
		Expiry: "12/30",
		CVV:    "123",
	}, "")

	out, err := formatPretty(info)
	require.NoError(t, err)

	s := string(out)
	assert.Contains(t, s, "4242424242424242")
	assert.Contains(t, s, "IVAN IVANOV")
	assert.Contains(t, s, "12/30")
	assert.Contains(t, s, "123")
}

func TestFormatPretty_Binary(t *testing.T) {
	info := makeInfo(t, domain.SecretTypeBinary, domain.BinarySecret{Data: []byte("hello world")}, "")

	out, err := formatPretty(info)
	require.NoError(t, err)

	s := string(out)
	assert.Contains(t, s, "Размер: 11 байт")
	assert.Contains(t, s, "--output")
}

func TestFormatPretty_InvalidJSON(t *testing.T) {
	types := []domain.SecretType{
		domain.SecretTypeCredentials,
		domain.SecretTypeText,
		domain.SecretTypeCard,
		domain.SecretTypeBinary,
	}
	for _, typ := range types {
		t.Run(string(typ), func(t *testing.T) {
			info := &domain.SecretInfo{
				SecretPayload: domain.SecretPayload{
					Name: "test", Type: typ, Data: json.RawMessage("not-json"),
				},
			}
			_, err := formatPretty(info)
			assert.Error(t, err)
		})
	}
}

func TestFormatPretty_UnknownType(t *testing.T) {
	info := &domain.SecretInfo{
		SecretPayload: domain.SecretPayload{
			Name: "test", Type: "unknown", Data: json.RawMessage("{}"),
		},
	}
	_, err := formatPretty(info)
	assert.Error(t, err)
}

func TestValidatePassword(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, validatePassword("password123"))
	})
	t.Run("too short", func(t *testing.T) {
		assert.ErrorIs(t, validatePassword("short"), ErrPasswordTooShort)
	})
	t.Run("empty", func(t *testing.T) {
		assert.ErrorIs(t, validatePassword(""), ErrPasswordTooShort)
	})
	t.Run("exactly min length", func(t *testing.T) {
		assert.NoError(t, validatePassword("12345678"))
	})
}
