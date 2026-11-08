package mobile

import (
	"bytes"
	"context"
	"strings"

	"github.com/F3dosik/Shron/internal/client/offline"
	"github.com/F3dosik/Shron/internal/client/vault"
	"github.com/F3dosik/Shron/internal/domain"
)

// Состояние записи относительно сервера (SecretSummary.Status).
const (
	StatusSynced   = string(vault.StatusSynced)
	StatusPending  = string(vault.StatusPending)
	StatusConflict = string(vault.StatusConflict)
)

// Причины конфликта (Conflict.Kind).
const (
	// ConflictChanged — запись изменили и на телефоне, и на сервере.
	ConflictChanged = string(offline.ConflictChanged)
	// ConflictDeletedOnServer — на телефоне изменили, на сервере удалили.
	ConflictDeletedOnServer = string(offline.ConflictDeletedOnServer)
	// ConflictExists — на телефоне создали, а на сервере уже есть запись с таким названием.
	ConflictExists = string(offline.ConflictExists)
	// ConflictChangedOnServer — на телефоне удалили, на сервере изменили.
	ConflictChangedOnServer = string(offline.ConflictChangedOnServer)
	// ConflictRejected — сервер не принял изменение (Conflict.Message — почему).
	ConflictRejected = string(offline.ConflictRejected)
)

// Решения по конфликту (ResolveConflict).
const (
	// ResolveMine — отправить версию с телефона.
	ResolveMine = string(vault.ResolveMine)
	// ResolveServer — оставить версию с сервера, изменение с телефона отбросить.
	ResolveServer = string(vault.ResolveServer)
	// ResolveBoth — оставить версию с сервера, а версию с телефона сохранить копией.
	ResolveBoth = string(vault.ResolveBoth)
)

// copyLabel — приписка к названию копии при ResolveBoth.
const copyLabel = "с телефона"

// Conflict — изменение, сделанное без сети, которое не удалось отправить.
// Время — миллисекунды Unix (0 — нет).
type Conflict struct {
	Name string
	Type string
	Kind string
	// EditedAt — когда запись изменили на телефоне.
	EditedAt int64
	// ServerUpdatedAt — когда запись изменили на сервере; 0 — на сервере её нет.
	ServerUpdatedAt int64
	// Message — ответ сервера для ConflictRejected.
	Message string
	// Differences — какие поля различаются, через запятую («пароль, сайты»).
	Differences string
	// CanKeepBoth — доступно ли ResolveBoth.
	CanKeepBoth bool
	// Mine и Server — версии на телефоне и на сервере; nil — записи там нет.
	Mine   *Secret
	Server *Secret
}

// ConflictList — конфликты: gomobile не передаёт срезы структур.
type ConflictList struct {
	items []*Conflict
}

// Len возвращает число конфликтов.
func (l *ConflictList) Len() int {
	return len(l.items)
}

// Get возвращает конфликт с индексом i или nil, если индекс вне списка.
func (l *ConflictList) Get(i int) *Conflict {
	if i < 0 || i >= len(l.items) {
		return nil
	}
	return l.items[i]
}

// Conflicts возвращает конфликты, ждущие решения пользователя.
func (a *App) Conflicts() (*ConflictList, error) {
	v, err := a.currentVault()
	if err != nil {
		return nil, err
	}
	conflicts, err := v.Conflicts()
	if err != nil {
		return nil, toError(err)
	}
	list := &ConflictList{}
	for _, c := range conflicts {
		item := &Conflict{
			Name:            c.Name,
			Type:            string(c.Type),
			Kind:            string(c.Kind),
			EditedAt:        unixMilli(c.EditedAt),
			ServerUpdatedAt: unixMilli(c.ServerUpdatedAt),
			Message:         c.Message,
			Differences:     strings.Join(differences(c.Mine, c.Server), ", "),
		}
		for _, r := range c.Resolutions {
			if r == vault.ResolveBoth {
				item.CanKeepBoth = true
			}
		}
		if item.Mine, err = payloadSecret(c.Mine); err != nil {
			return nil, err
		}
		if item.Server, err = payloadSecret(c.Server); err != nil {
			return nil, err
		}
		list.items = append(list.items, item)
	}
	return list, nil
}

// ResolveConflict разрешает конфликт записи: choice — ResolveMine, ResolveServer или
// ResolveBoth (копия называется «название (с телефона, дата)»).
func (a *App) ResolveConflict(name, secretType, choice string) error {
	v, err := a.currentVault()
	if err != nil {
		return err
	}
	t, err := parseType(secretType)
	if err != nil {
		return err
	}
	return toError(v.ResolveConflict(context.Background(), name, t, vault.Resolution(choice), copyLabel))
}

func payloadSecret(p *domain.SecretPayload) (*Secret, error) {
	if p == nil {
		return nil, nil
	}
	return toSecret(&domain.SecretInfo{SecretPayload: *p})
}

// differences — названия различающихся полей двух версий записи.
func differences(mine, server *domain.SecretPayload) []string {
	if mine == nil || server == nil {
		return nil
	}
	var diff []string
	add := func(label string, a, b any) {
		if a != b {
			diff = append(diff, label)
		}
	}
	m, err1 := toSecret(&domain.SecretInfo{SecretPayload: *mine})
	s, err2 := toSecret(&domain.SecretInfo{SecretPayload: *server})
	if err1 != nil || err2 != nil {
		return []string{"содержимое"}
	}
	switch mine.Type {
	case domain.SecretTypeCredentials:
		add("логин", m.Login, s.Login)
		add("пароль", m.Password, s.Password)
		add("сайты и приложения", m.URIs, s.URIs)
	case domain.SecretTypeText:
		add("текст", m.Text, s.Text)
	case domain.SecretTypeCard:
		add("номер карты", m.CardNumber, s.CardNumber)
		add("держатель", m.CardHolder, s.CardHolder)
		add("срок действия", m.CardExpiry, s.CardExpiry)
		add("CVV", m.CardCVV, s.CardCVV)
	case domain.SecretTypeBinary:
		if !bytes.Equal(mine.Data, server.Data) {
			diff = append(diff, "файл")
		}
	}
	add("комментарий", m.Metadata, s.Metadata)
	add("папка", m.Group, s.Group)
	return diff
}
