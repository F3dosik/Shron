//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"

	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/mobile"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OnLock делает lockRecorder реализацией mobile.LockListener (в приложении — код на Kotlin).
func (r *lockRecorder) OnLock(reason string) {
	r.record(vault.LockReason(reason))
}

func newMobileApp(t *testing.T, dir string, listener mobile.LockListener) *mobile.App {
	t.Helper()
	app, err := mobile.NewApp(dir, true, listener)
	require.NoError(t, err)
	t.Cleanup(app.Close)
	return app
}

// mobileCode достаёт код из ошибки, как это делает приложение на Kotlin.
func mobileCode(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	var e mobile.Error
	require.NoError(t, json.Unmarshal([]byte(err.Error()), &e), "error must be JSON: %s", err)
	assert.NotEmpty(t, e.Message)
	return e.Code
}

func TestE2E_Mobile_FullFlow(t *testing.T) {
	dir := t.TempDir()
	locks := &lockRecorder{}
	app := newMobileApp(t, dir, locks)

	msg, err := app.CheckServer(serverAddr, "", false)
	require.NoError(t, err)
	assert.Contains(t, msg, "Сервер доступен")
	require.NoError(t, app.SaveSettings(serverAddr, "", false, 5))

	login := "mob-" + uuid.NewString()
	require.NoError(t, app.Register(login, "mobile-pass-1"))
	changeRequired, err := app.SignIn(login, "mobile-pass-1")
	require.NoError(t, err)
	assert.False(t, changeRequired)
	assert.True(t, app.GetState().Unlocked)

	require.NoError(t, app.SaveSecret(&mobile.SecretInput{Name: "github", Type: mobile.TypeCredentials, Login: "me", Password: "pw-1"}, false))
	require.NoError(t, app.SaveSecret(&mobile.SecretInput{
		Name: "visa", Type: mobile.TypeCard, CardNumber: "4111 1111 1111 1111", CardHolder: "IVAN IVANOV", CardExpiry: "12/30", CardCVV: "123",
	}, false))
	require.NoError(t, app.SaveSecret(&mobile.SecretInput{Name: "key.bin", Type: mobile.TypeBinary, FileData: []byte{0, 1, 2, 255}}, false))
	assert.Equal(t, mobile.CodeAlreadyExists, mobileCode(t, app.SaveSecret(&mobile.SecretInput{Name: "github", Type: mobile.TypeCredentials, Login: "x"}, false)))

	list, err := app.ListSecrets()
	require.NoError(t, err)
	assert.Equal(t, 3, list.Len())
	assert.NotZero(t, list.Get(0).CreatedAt)

	cred, err := app.GetSecret("github", mobile.TypeCredentials)
	require.NoError(t, err)
	assert.Equal(t, "pw-1", cred.Password)

	// Изменение файла без нового содержимого сохраняет прежнее.
	require.NoError(t, app.SaveSecret(&mobile.SecretInput{Name: "key.bin", Type: mobile.TypeBinary, Metadata: "ssh"}, true))
	file, err := app.GetSecret("key.bin", mobile.TypeBinary)
	require.NoError(t, err)
	assert.Equal(t, 4, file.FileSize)
	assert.Equal(t, "ssh", file.Metadata)
	content, err := app.GetFileContent("key.bin")
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 1, 2, 255}, content)

	app.Lock()
	assert.Equal(t, mobile.LockManual, string(locks.last()))
	_, err = app.ListSecrets()
	assert.Equal(t, mobile.CodeLocked, mobileCode(t, err))
	assert.Equal(t, mobile.CodeWrongPassword, mobileCode(t, app.Unlock("wrong-password-1")))
	require.NoError(t, app.Unlock("mobile-pass-1"))

	require.NoError(t, app.DeleteSecret("github", mobile.TypeCredentials))
	_, err = app.GetSecret("github", mobile.TypeCredentials)
	assert.Equal(t, mobile.CodeNotFound, mobileCode(t, err))

	// После перезапуска логин помнится, ключи — нет.
	app.Close()
	restarted := newMobileApp(t, dir, nil)
	state := restarted.GetState()
	assert.Equal(t, login, state.Login)
	assert.False(t, state.Unlocked)
	require.NoError(t, restarted.Unlock("mobile-pass-1"))

	require.NoError(t, restarted.Logout(false))
	assert.Empty(t, restarted.GetState().Login)
}

func TestE2E_Mobile_UnlockWithKey(t *testing.T) {
	app := newMobileApp(t, t.TempDir(), nil)
	require.NoError(t, app.SaveSettings(serverAddr, "", false, 5))

	login := "mob-key-" + uuid.NewString()
	require.NoError(t, app.Register(login, "mobile-pass-1"))
	_, err := app.SignIn(login, "mobile-pass-1")
	require.NoError(t, err)
	require.NoError(t, app.SaveSecret(&mobile.SecretInput{Name: "note", Type: mobile.TypeText, Text: "hello"}, false))

	key, err := app.MasterKey()
	require.NoError(t, err)
	require.Len(t, key, 32)

	// Заблокированное хранилище ключ не отдаёт, а сохранённым ключом открывается.
	app.Lock()
	_, err = app.MasterKey()
	assert.Equal(t, mobile.CodeLocked, mobileCode(t, err))
	require.NoError(t, app.UnlockWithKey(key))
	assert.True(t, app.GetState().Unlocked)
	note, err := app.GetSecret("note", mobile.TypeText)
	require.NoError(t, err)
	assert.Equal(t, "hello", note.Text)

	// Чужой ключ сервер не принимает, хранилище остаётся заблокированным.
	wrong := make([]byte, len(key))
	assert.Equal(t, mobile.CodeKeyRejected, mobileCode(t, app.UnlockWithKey(wrong)))
	assert.False(t, app.GetState().Unlocked)

	// После смены пароля прежний ключ не годится.
	_, err = app.SignIn(login, "mobile-pass-1")
	require.NoError(t, err)
	require.NoError(t, app.ChangePassword("mobile-pass-1", "mobile-pass-2", 3, 64))
	app.Lock()
	assert.Equal(t, mobile.CodeKeyRejected, mobileCode(t, app.UnlockWithKey(key)))

	// Без учётки разблокировать нечего.
	_, err = app.SignIn(login, "mobile-pass-2")
	require.NoError(t, err)
	require.NoError(t, app.Logout(false))
	assert.Equal(t, mobile.CodeNotSignedIn, mobileCode(t, app.UnlockWithKey(key)))
}
