package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettings_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	settings, err := loadSettings(dir)
	require.NoError(t, err)
	assert.Equal(t, Settings{AutoLockMinutes: defaultAutoLockMinutes}, settings, "defaults without a file")

	want := Settings{ServerAddress: "host:50051", AutoLockMinutes: 3}
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

	// Пустой сертификат удаляет файл: тогда используются системные корневые сертификаты.
	require.NoError(t, saveSettings(dir, want, nil))
	ca, err = loadCACert(dir)
	require.NoError(t, err)
	assert.Empty(t, ca)
}

func TestToUIError(t *testing.T) {
	decode := func(err error) Error {
		var e Error
		require.NoError(t, json.Unmarshal([]byte(err.Error()), &e))
		return e
	}

	assert.NoError(t, toUIError(nil))

	e := decode(toUIError(fmt.Errorf("unlock: %w", vault.ErrWrongPassword)))
	assert.Equal(t, vault.CodeWrongPassword, e.Code)
	assert.Equal(t, "Неверный мастер-пароль", e.Message)

	e = decode(toUIError(fmt.Errorf("%w: too many requests, try again later", domain.ErrResourceExhausted)))
	assert.Equal(t, vault.CodeLimitExceeded, e.Code)
	assert.Contains(t, e.Message, "too many requests", "server details are kept when there is no fixed message")

	e = decode(toUIError(domain.ErrInvalidCardExpiry))
	assert.Equal(t, CodeValidation, e.Code)

	original := validationError("x")
	assert.Same(t, original, toUIError(original), "already converted errors pass through")
}

func TestValidators(t *testing.T) {
	addr, err := normalizeAddress("192.168.1.5")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.5:50051", addr, "port is optional")
	addr, err = normalizeAddress("192.168.1.5:6000")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.5:6000", addr)
	_, err = normalizeAddress("")
	assert.Error(t, err)

	assert.Error(t, checkCACert([]byte("not a certificate")))

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

func TestResolveCACert(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(Options{Dir: dir})
	require.NoError(t, saveSettings(dir, Settings{ServerAddress: "h:1"}, []byte("SAVED")))

	ca, err := app.resolveCACert("", true)
	require.NoError(t, err)
	assert.Equal(t, []byte("SAVED"), ca, "saved certificate is reused without the UI knowing it")

	ca, err = app.resolveCACert("", false)
	require.NoError(t, err)
	assert.Nil(t, ca, "empty means system roots")

	_, err = app.resolveCACert("garbage", false)
	assert.Error(t, err)
}

func TestGenerate(t *testing.T) {
	app := NewApp(Options{Dir: t.TempDir()})

	g, err := app.GeneratePassword(GeneratorOptions{Length: 24, Lower: true, Digits: true})
	require.NoError(t, err)
	assert.Regexp(t, `^[a-z0-9]{24}$`, g.Password)
	assert.Equal(t, 124, g.EntropyBits) // 24·log2(36) ≈ 124.1 минус малая поправка на «есть цифра и буква»
	assert.NotEmpty(t, g.Strength)

	p, err := app.GeneratePassphrase(6)
	require.NoError(t, err)
	assert.Equal(t, 77, p.EntropyBits)

	_, err = app.GeneratePassword(GeneratorOptions{Length: 20})
	assert.ErrorContains(t, err, "VALIDATION")
	_, err = app.GeneratePassphrase(2)
	assert.ErrorContains(t, err, "VALIDATION")
}

func TestGroup_RoundTrip(t *testing.T) {
	app := &App{}
	p, err := app.buildPayload(SecretInput{Name: "x", Type: string(domain.SecretTypeText), Text: "t", Group: " Дом  и семья "}, false)
	require.NoError(t, err)
	assert.Equal(t, "Дом и семья", p.Group)

	s, err := toSecret(&domain.SecretInfo{SecretPayload: *p})
	require.NoError(t, err)
	assert.Equal(t, "Дом и семья", s.Group)

	_, err = app.buildPayload(SecretInput{
		Name: "x", Type: string(domain.SecretTypeText), Text: "t", Group: strings.Repeat("я", domain.MaxGroupLength+1),
	}, false)
	assert.ErrorContains(t, err, "VALIDATION")
}

func TestCredentialURIs_RoundTrip(t *testing.T) {
	app := &App{}
	p, err := app.buildPayload(SecretInput{
		Name: "github", Type: string(domain.SecretTypeCredentials), Login: "me", Password: "pw",
		URIs: "github.com\n\nandroidapp://com.github.android",
	}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"login":"me","password":"pw","uris":["github.com","androidapp://com.github.android"]}`, string(p.Data))

	s, err := toSecret(&domain.SecretInfo{SecretPayload: *p})
	require.NoError(t, err)
	assert.Equal(t, "github.com\nandroidapp://com.github.android", s.URIs)

	// Без сайтов данные секрета не меняются по сравнению с прежним форматом.
	p, err = app.buildPayload(SecretInput{Name: "x", Type: string(domain.SecretTypeCredentials), Login: "me", Password: "pw"}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"login":"me","password":"pw"}`, string(p.Data))
}
