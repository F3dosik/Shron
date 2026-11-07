package command_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/client/command"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	fn()

	w.Close()
	os.Stdout = orig
	return <-done
}

func credentialsInfo(t *testing.T) *domain.SecretInfo {
	t.Helper()
	data, err := json.Marshal(domain.CredentialsSecret{Login: "ivan", Password: "s3cret"})
	require.NoError(t, err)
	return &domain.SecretInfo{
		SecretPayload: domain.SecretPayload{
			Name: "github", Type: domain.SecretTypeCredentials, Data: data,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestWriteSecret_JSONToFile(t *testing.T) {
	info := credentialsInfo(t)
	path := filepath.Join(t.TempDir(), "out.json")

	err := command.WriteSecret(info, command.OutputOptions{JSON: true, OutputPath: path})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got domain.SecretInfo
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, info.Name, got.Name)
	assert.Equal(t, info.Type, got.Type)
}

func TestWriteSecret_PrettyToFile(t *testing.T) {
	info := credentialsInfo(t)
	path := filepath.Join(t.TempDir(), "out.txt")

	err := command.WriteSecret(info, command.OutputOptions{OutputPath: path})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "ivan")
	assert.Contains(t, string(data), "s3cret")
}

func TestWriteSecret_PrettyShowsURIs(t *testing.T) {
	info := credentialsInfo(t)
	path := filepath.Join(t.TempDir(), "out.txt")
	require.NoError(t, command.WriteSecret(info, command.OutputOptions{OutputPath: path}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "Сайты:", "no line for secrets without sites")

	info.Data, err = json.Marshal(domain.CredentialsSecret{
		Login: "ivan", Password: "s3cret", URIs: []string{"github.com", "androidapp://com.github.android"},
	})
	require.NoError(t, err)
	require.NoError(t, command.WriteSecret(info, command.OutputOptions{OutputPath: path}))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Сайты:    github.com, androidapp://com.github.android")
}

func TestWriteSecret_BinaryRawToFile(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02, 0x03, 0xff}
	data, err := json.Marshal(domain.BinarySecret{Data: raw})
	require.NoError(t, err)
	info := &domain.SecretInfo{
		SecretPayload: domain.SecretPayload{Name: "file", Type: domain.SecretTypeBinary, Data: data},
	}
	path := filepath.Join(t.TempDir(), "out.bin")

	err = command.WriteSecret(info, command.OutputOptions{OutputPath: path})
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, raw, got, "binary content should be written raw, not JSON-encoded")
}

func TestWriteSecret_BinaryInvalidJSON(t *testing.T) {
	info := &domain.SecretInfo{
		SecretPayload: domain.SecretPayload{
			Name: "file", Type: domain.SecretTypeBinary, Data: json.RawMessage("not-json"),
		},
	}
	path := filepath.Join(t.TempDir(), "out.bin")

	err := command.WriteSecret(info, command.OutputOptions{OutputPath: path})
	assert.Error(t, err)
}

func TestWriteSecret_PrettyToStdout(t *testing.T) {
	info := credentialsInfo(t)

	out := captureStdout(t, func() {
		err := command.WriteSecret(info, command.OutputOptions{})
		require.NoError(t, err)
	})

	assert.Contains(t, out, "ivan")
	assert.Contains(t, out, "s3cret")
}

func TestWriteSecretList_All(t *testing.T) {
	secrets := []*domain.SecretInfo{
		{SecretPayload: domain.SecretPayload{Name: "a", Type: domain.SecretTypeCredentials, Metadata: "m1"}, UpdatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{SecretPayload: domain.SecretPayload{Name: "b", Type: domain.SecretTypeText}, UpdatedAt: time.Date(2024, 2, 1, 11, 0, 0, 0, time.UTC)},
	}

	out := captureStdout(t, func() {
		err := command.WriteSecretList(secrets, command.ListFilter{})
		require.NoError(t, err)
	})

	assert.Contains(t, out, "НАЗВАНИЕ")
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "b")
	assert.Contains(t, out, "m1")
}

func TestWriteSecretList_Filter(t *testing.T) {
	secrets := []*domain.SecretInfo{
		{SecretPayload: domain.SecretPayload{Name: "creds", Type: domain.SecretTypeCredentials}},
		{SecretPayload: domain.SecretPayload{Name: "note", Type: domain.SecretTypeText}},
	}

	out := captureStdout(t, func() {
		err := command.WriteSecretList(secrets, command.ListFilter{Type: "text"})
		require.NoError(t, err)
	})

	assert.Contains(t, out, "note")
	assert.NotContains(t, out, "creds")
}

func TestWriteSecretList_Groups(t *testing.T) {
	secrets := []*domain.SecretInfo{
		{SecretPayload: domain.SecretPayload{Name: "loose", Type: domain.SecretTypeText}},
		{SecretPayload: domain.SecretPayload{Name: "mail", Type: domain.SecretTypeCredentials, Group: "Работа"}},
		{SecretPayload: domain.SecretPayload{Name: "bank", Type: domain.SecretTypeCard, Group: "Банки"}},
	}

	out := captureStdout(t, func() {
		require.NoError(t, command.WriteSecretList(secrets, command.ListFilter{}))
	})
	// По группам в алфавитном порядке, без группы — в конце.
	assert.Less(t, strings.Index(out, "bank"), strings.Index(out, "mail"))
	assert.Less(t, strings.Index(out, "mail"), strings.Index(out, "loose"))

	out = captureStdout(t, func() {
		require.NoError(t, command.WriteSecretList(secrets, command.ListFilter{Group: " работа "}))
	})
	assert.Contains(t, out, "mail")
	assert.NotContains(t, out, "bank")
	assert.NotContains(t, out, "loose")
}

func TestWriteSecretList_InvalidFilter(t *testing.T) {
	err := command.WriteSecretList(nil, command.ListFilter{Type: "bogus"})
	assert.ErrorIs(t, err, domain.ErrUnknownSecretType)
}

func TestWriteSecretList_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		err := command.WriteSecretList(nil, command.ListFilter{})
		require.NoError(t, err)
	})

	assert.Contains(t, out, "НАЗВАНИЕ")
}
