package domain_test

import (
	"strings"
	"testing"

	"github.com/F3dosik/Shron/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGroup(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"only spaces", " \t\n ", ""},
		{"trimmed", "  Работа ", "Работа"},
		{"inner spaces collapsed", "Дом \n  и   семья", "Дом и семья"},
		{"case kept", "Банки", "Банки"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NormalizeGroup(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeGroup_Length(t *testing.T) {
	// Длина считается в символах, а не в байтах: кириллица занимает по два байта.
	longest := strings.Repeat("я", domain.MaxGroupLength)
	got, err := domain.NormalizeGroup(longest)
	require.NoError(t, err)
	assert.Equal(t, longest, got)

	_, err = domain.NormalizeGroup(longest + "я")
	assert.ErrorIs(t, err, domain.ErrGroupTooLong)
}
