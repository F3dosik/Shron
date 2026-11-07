package vault

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/F3dosik/Shron/internal/client/service"
	"github.com/F3dosik/Shron/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorCode(t *testing.T) {
	cases := map[error]Code{
		nil:                             "",
		ErrWrongPassword:                CodeWrongPassword,
		ErrSessionExpired:               CodeSessionExpired,
		ErrKeyRejected:                  CodeKeyRejected,
		fmt.Errorf("op: %w", ErrLocked): CodeLocked,
		fmt.Errorf("%w: too many requests", domain.ErrResourceExhausted): CodeLimitExceeded,
		fmt.Errorf("get: %w", service.ErrIntegrity):                      CodeIntegrity,
		fmt.Errorf("%w: connection refused", domain.ErrUnavailable):      CodeUnavailable,
		fmt.Errorf("%w: tls: failed to verify certificate: x509: certificate signed by unknown authority",
			domain.ErrUnavailable): CodeBadCertificate,
		errors.New("something else"): CodeInternal,
	}
	for err, want := range cases {
		assert.Equal(t, want, ErrorCode(err), "%v", err)
	}
}

func TestTokenExpiry(t *testing.T) {
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(exp),
	}).SignedString([]byte("any-key"))
	require.NoError(t, err)

	got, ok := tokenExpiry(token)
	require.True(t, ok)
	assert.True(t, exp.Equal(got))

	_, ok = tokenExpiry("")
	assert.False(t, ok)
	_, ok = tokenExpiry("not-a-jwt")
	assert.False(t, ok)
}
