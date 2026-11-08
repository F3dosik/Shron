package mobile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeError(t *testing.T, err error) Error {
	t.Helper()
	require.Error(t, err)
	var e Error
	require.NoError(t, json.Unmarshal([]byte(err.Error()), &e), "error must be JSON: %s", err)
	return e
}

func TestNewApp_NotConfigured(t *testing.T) {
	_, err := NewApp("", true, nil)
	assert.Equal(t, CodeValidation, decodeError(t, err).Code)

	app, err := NewApp(filepath.Join(t.TempDir(), "data"), true, nil)
	require.NoError(t, err)
	t.Cleanup(app.Close)

	state := app.GetState()
	assert.False(t, state.Configured)
	assert.Equal(t, defaultAutoLockMinutes, state.AutoLockMinutes)
	_, err = app.SignIn("user", "password-1")
	assert.Equal(t, CodeNotConfigured, decodeError(t, err).Code)
}

// Сломанные сохранённые настройки не мешают запуску: их можно исправить.
func TestNewApp_BrokenSessionCanBeFixed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, saveSettings(dir, settings{ServerAddress: "127.0.0.1:1"}, nil))
	require.NoError(t, os.WriteFile(filepath.Join(dir, sessionFile), []byte("{broken"), 0o600))

	app, err := NewApp(dir, true, nil)
	require.NoError(t, err)
	t.Cleanup(app.Close)
	assert.True(t, app.GetState().Configured)
	assert.Equal(t, CodeInternal, decodeError(t, app.Unlock("password-1")).Code)

	// Смена сервера удаляет сессию старого сервера, и подключение восстанавливается.
	require.NoError(t, app.SaveSettings("127.0.0.1:2", "", false, 5))
	assert.Equal(t, CodeNotSignedIn, decodeError(t, app.Unlock("password-1")).Code)
}

func TestSettings_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	s, err := loadSettings(dir)
	require.NoError(t, err)
	assert.Equal(t, settings{AutoLockMinutes: defaultAutoLockMinutes}, s, "defaults without a file")

	want := settings{ServerAddress: "host:50051", AutoLockMinutes: 3}
	require.NoError(t, saveSettings(dir, want, []byte("PEM")))
	got, err := loadSettings(dir)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	ca, err := loadCACert(dir)
	require.NoError(t, err)
	assert.Equal(t, []byte("PEM"), ca)
	info, err := os.Stat(filepath.Join(dir, caCertFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, saveSettings(dir, want, nil))
	ca, err = loadCACert(dir)
	require.NoError(t, err)
	assert.Empty(t, ca)
}

func TestSaveSettings_Validation(t *testing.T) {
	app, err := NewApp(t.TempDir(), true, nil)
	require.NoError(t, err)
	t.Cleanup(app.Close)

	assert.Equal(t, CodeValidation, decodeError(t, app.SaveSettings("http://bad", "", false, 5)).Code)
	assert.Equal(t, CodeValidation, decodeError(t, app.SaveSettings("host", "not a cert", false, 5)).Code)
	assert.Equal(t, CodeValidation, decodeError(t, app.SaveSettings("host", "", false, -1)).Code)

	require.NoError(t, app.SaveSettings("192.0.2.10", "", false, 7))
	state := app.GetState()
	assert.Equal(t, "192.0.2.10:50051", state.ServerAddress, "port is optional")
	assert.Equal(t, 7, state.AutoLockMinutes)
	assert.False(t, state.HasCACert)
}

func TestToError(t *testing.T) {
	assert.NoError(t, toError(nil))

	e := decodeError(t, toError(fmt.Errorf("unlock: %w", vault.ErrWrongPassword)))
	assert.Equal(t, CodeWrongPassword, e.Code)
	assert.Equal(t, "Неверный мастер-пароль", e.Message)

	e = decodeError(t, toError(fmt.Errorf("%w: too many requests", domain.ErrResourceExhausted)))
	assert.Equal(t, CodeLimitExceeded, e.Code)
	assert.Contains(t, e.Message, "too many requests", "server details are kept when there is no fixed message")

	e = decodeError(t, toError(&domain.KDFDowngradeError{Known: domain.DefaultKDFParams, Received: domain.LegacyKDFParams}))
	assert.Equal(t, CodeKDFDowngrade, e.Code)
	assert.Contains(t, e.Message, "Argon2id")

	assert.Equal(t, CodeValidation, decodeError(t, toError(domain.ErrInvalidCardExpiry)).Code)

	original := validationError("x")
	assert.Same(t, original, toError(original), "already converted errors pass through")
}

func TestRegistrationError(t *testing.T) {
	assert.NoError(t, registrationError(nil))

	e := decodeError(t, registrationError(fmt.Errorf("%w: registration is disabled", domain.ErrPermissionDenied)))
	assert.Equal(t, CodePermissionDenied, e.Code)
	assert.Equal(t, registrationClosedMessage, e.Message)

	assert.Equal(t, CodeAlreadyExists, decodeError(t, registrationError(domain.ErrAlreadyExists)).Code)
}

func TestValidators(t *testing.T) {
	assert.Error(t, checkCredentials(" ", "long-enough"))
	assert.Error(t, checkCredentials("user", "short"))
	assert.NoError(t, checkCredentials("user", "long-enough"))

	assert.Error(t, checkNewPassword("same-password", "same-password"))
	assert.NoError(t, checkNewPassword("old-password", "new-password"))

	params, err := kdfParams(4, 128)
	require.NoError(t, err)
	assert.Equal(t, domain.KDFParams{Time: 4, MemoryKiB: 128 * 1024, Threads: domain.DefaultKDFParams.Threads}, params)
	for _, bad := range [][2]int{{0, 64}, {11, 64}, {3, 8}, {3, 4096}, {3, -1}} {
		_, err := kdfParams(bad[0], bad[1])
		assert.Error(t, err, "%v", bad)
	}
}

func TestBuildPayload(t *testing.T) {
	app := &App{}

	p, err := app.buildPayload(&SecretInput{Name: " github ", Type: TypeCredentials, Login: "me", Password: "pw"}, false)
	require.NoError(t, err)
	assert.Equal(t, "github", p.Name)
	assert.JSONEq(t, `{"login":"me","password":"pw"}`, string(p.Data))

	p, err = app.buildPayload(&SecretInput{
		Name: "github", Type: TypeCredentials, Login: "me", Password: "pw",
		URIs: "github.com\n androidapp://com.github.android, github.com",
	}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"login":"me","password":"pw","uris":["github.com","androidapp://com.github.android"]}`, string(p.Data))

	p, err = app.buildPayload(&SecretInput{Name: "x", Type: TypeText, Text: "t", Group: "  Работа \n почта "}, false)
	require.NoError(t, err)
	assert.Equal(t, "Работа почта", p.Group)

	p, err = app.buildPayload(&SecretInput{Name: "key", Type: TypeBinary, FileData: []byte{0, 255}}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"data":"AP8="}`, string(p.Data))

	for _, bad := range []*SecretInput{
		{Type: TypeText, Text: "x"},
		{Name: "x", Type: "unknown"},
		{Name: "x", Type: TypeCredentials},
		{Name: "x", Type: TypeText},
		{Name: "x", Type: TypeCard, CardNumber: "1234", CardHolder: "X", CardExpiry: "12/30", CardCVV: "123"},
		{Name: "x", Type: TypeBinary},
		{Name: "x", Type: TypeBinary, FileData: make([]byte, maxFileSize+1)},
		{Name: "x", Type: TypeText, Text: "t", Group: strings.Repeat("я", domain.MaxGroupLength+1)},
	} {
		_, err := app.buildPayload(bad, false)
		assert.Equal(t, CodeValidation, decodeError(t, err).Code, "%+v", bad.Name+"/"+bad.Type)
	}
}

func TestToSecret(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s, err := toSecret(&domain.SecretInfo{
		SecretPayload: domain.SecretPayload{
			Name: "visa", Type: domain.SecretTypeCard, Metadata: "m", Group: "Банки",
			Data: json.RawMessage(`{"number":"4111","holder":"H","expiry":"12/30","cvv":"123"}`),
		},
		CreatedAt: created,
	})
	require.NoError(t, err)
	assert.Equal(t, &Secret{
		Name: "visa", Type: TypeCard, Metadata: "m", Group: "Банки", CreatedAt: created.UnixMilli(),
		CardNumber: "4111", CardHolder: "H", CardExpiry: "12/30", CardCVV: "123",
	}, s)

	s, err = toSecret(&domain.SecretInfo{SecretPayload: domain.SecretPayload{
		Type: domain.SecretTypeCredentials,
		Data: json.RawMessage(`{"login":"me","password":"pw","uris":["github.com","androidapp://com.github.android"]}`),
	}})
	require.NoError(t, err)
	assert.Equal(t, "github.com\nandroidapp://com.github.android", s.URIs)

	s, err = toSecret(&domain.SecretInfo{SecretPayload: domain.SecretPayload{
		Type: domain.SecretTypeBinary, Data: json.RawMessage(`{"data":"AAEC"}`),
	}})
	require.NoError(t, err)
	assert.Equal(t, 3, s.FileSize)
}

func TestSecretList(t *testing.T) {
	list := &SecretList{items: []*SecretSummary{{Name: "a"}, {Name: "b"}}}
	assert.Equal(t, 2, list.Len())
	assert.Equal(t, "b", list.Get(1).Name)
	assert.Nil(t, list.Get(2))
	assert.Nil(t, list.Get(-1))
}

func TestGenerate(t *testing.T) {
	g, err := GeneratePassword(&GeneratorOptions{Length: 24, Lower: true, Digits: true})
	require.NoError(t, err)
	assert.Regexp(t, `^[a-z0-9]{24}$`, g.Password)
	assert.NotEmpty(t, g.Strength)

	p, err := GeneratePassphrase(6)
	require.NoError(t, err)
	assert.Equal(t, 77, p.EntropyBits)

	_, err = GeneratePassword(&GeneratorOptions{Length: 20})
	assert.Equal(t, CodeValidation, decodeError(t, err).Code)
	_, err = GeneratePassword(nil)
	assert.Equal(t, CodeValidation, decodeError(t, err).Code)
	_, err = GeneratePassphrase(2)
	assert.Equal(t, CodeValidation, decodeError(t, err).Code)
}

func TestApp_Standalone(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir, true, nil)
	require.NoError(t, err)
	t.Cleanup(app.Close)
	assert.False(t, app.GetState().Configured)

	require.NoError(t, app.UseStandalone(5))
	state := app.GetState()
	assert.True(t, state.Configured)
	assert.True(t, state.Standalone)
	assert.False(t, state.HasStandaloneVault)

	require.Error(t, app.CreateStandalone("short"))
	require.NoError(t, app.CreateStandalone("password-1"))
	state = app.GetState()
	assert.True(t, state.HasStandaloneVault)
	assert.True(t, state.Unlocked)
	assert.Empty(t, state.Login)

	require.NoError(t, app.SaveSecret(&SecretInput{Name: "note", Type: TypeText, Text: "hello"}, false))
	list, err := app.ListSecrets()
	require.NoError(t, err)
	require.Equal(t, 1, list.Len())

	// Настройки переживают перезапуск.
	app.Close()
	app, err = NewApp(dir, true, nil)
	require.NoError(t, err)
	assert.True(t, app.GetState().Standalone)
	require.NoError(t, app.Unlock("password-1"))
	s, err := app.GetSecret("note", TypeText)
	require.NoError(t, err)
	assert.Equal(t, "hello", s.Text)

	err = app.Logout(false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeNotSupported)
}
