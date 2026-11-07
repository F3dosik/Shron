package domain_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURIs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"only separators", " ,\n; \t", nil},
		{"one per line", "github.com\nandroidapp://com.github.android\n", []string{"github.com", "androidapp://com.github.android"}},
		{"commas and spaces", "github.com, gitlab.com  bitbucket.org", []string{"github.com", "gitlab.com", "bitbucket.org"}},
		{"windows line breaks", "a.ru\r\nb.ru", []string{"a.ru", "b.ru"}},
		{"duplicates ignoring case", "GitHub.com github.com https://github.com/login", []string{"GitHub.com", "https://github.com/login"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, domain.ParseURIs(tt.in))
		})
	}
}

func TestParseURIs_Limit(t *testing.T) {
	var b strings.Builder
	for i := 0; i < domain.MaxCredentialURIs+5; i++ {
		fmt.Fprintf(&b, "site%d.com\n", i)
	}
	assert.Len(t, domain.ParseURIs(b.String()), domain.MaxCredentialURIs)
}

func TestCredentialsSecret_URIsAreOptional(t *testing.T) {
	// Секреты, созданные до появления поля, читаются без него.
	var old domain.CredentialsSecret
	require.NoError(t, json.Unmarshal([]byte(`{"login":"me","password":"pw"}`), &old))
	assert.Empty(t, old.URIs)

	// Без сайтов поле не пишется: данные таких секретов не меняются.
	data, err := json.Marshal(domain.CredentialsSecret{Login: "me", Password: "pw"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"login":"me","password":"pw"}`, string(data))

	data, err = json.Marshal(domain.CredentialsSecret{Login: "me", Password: "pw", URIs: []string{"github.com"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"login":"me","password":"pw","uris":["github.com"]}`, string(data))
}

func TestMatchURI_Sites(t *testing.T) {
	tests := []struct {
		uri, site string
		want      bool
	}{
		{"github.com", "github.com", true},
		{"github.com", "gist.github.com", true},
		{"https://github.com/login", "github.com", true},
		{"GitHub.com", "WWW.GITHUB.COM", true},
		{"www.github.com", "github.com", true},
		{"https://user:pw@github.com:8443/x?y#z", "github.com", true},
		{"github.com.", "github.com", true},
		{"192.168.1.5", "192.168.1.5", true},
		{"http://[::1]:8080/", "::1", true},
		{"localhost:3000", "localhost", true},

		// Фишинг и чужие домены.
		{"github.com", "github.com.evil.ru", false},
		{"github.com", "notgithub.com", false},
		{"gist.github.com", "github.com", false},
		{"com", "github.com", false},
		{"github", "github.com", false},
		{"", "github.com", false},
		{"github.com", "", false},
		{"androidapp://com.github.android", "github.com", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, domain.MatchURI(tt.uri, tt.site, "com.android.chrome"), "%q on %q", tt.uri, tt.site)
	}
}

func TestMatchURI_Apps(t *testing.T) {
	assert.True(t, domain.MatchURI("androidapp://com.github.android", "", "com.github.android"))
	assert.True(t, domain.MatchURI("AndroidApp://Com.GitHub.Android ", "", "com.github.android"))
	assert.False(t, domain.MatchURI("androidapp://com.github.android", "", "com.github.android.evil"))
	assert.False(t, domain.MatchURI("androidapp://", "", ""))
	// В браузере пакет — сам браузер, а не сайт.
	assert.False(t, domain.MatchURI("androidapp://com.android.chrome", "github.com", "com.android.chrome"))
	// Сайт не подходит приложению, даже если имя похоже.
	assert.False(t, domain.MatchURI("github.com", "", "com.github.android"))
}
