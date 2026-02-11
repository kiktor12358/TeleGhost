package appcore

import (
	"fmt"
	"time"

	"teleghost/internal/core"

	"github.com/google/uuid"
)

// GetContacts возвращает список контактов с последним сообщением.
func (a *AppCore) GetContacts() ([]*ContactInfo, error) {
	if a.Repo == nil {
		return []*ContactInfo{}, nil
	}

	contacts, err := a.Repo.ListContactsWithLastMessage(a.Ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*ContactInfo, len(contacts))
	for i, c := range contacts {
		info := &ContactInfo{
			ID:          c.ID,
			Nickname:    c.Nickname,
			Bio:         c.Bio,
			Avatar:      a.formatAvatarURL(c.Avatar),
			I2PAddress:  c.I2PAddress,
			PublicKey:   c.PublicKey,
			ChatID:      c.ChatID,
			IsBlocked:   c.IsBlocked,
			IsVerified:  c.IsVerified,
			UnreadCount: c.UnreadCount,
		}
		if c.LastMessage != "" {
			info.LastMessage = c.LastMessage
			// Можно добавить форматирование времени
			tm := c.LastMessageTime
			info.LastMessageTime = &tm
		}
		result[i] = info
	}

	return result, nil
}

// AddContact добавляет контакт по адресу.
func (a *AppCore) AddContact(name, destination string) (*ContactInfo, error) {
	if a.Repo == nil {
		return nil, fmt.Errorf("not logged in")
	}

	// Минимальная валидация адреса
	if len(destination) < 32 {
		return nil, fmt.Errorf("некорректный I2P адрес (слишком короткий)")
	}

	contact := &core.Contact{
		ID:         uuid.New().String(),
		Nickname:   name,
		I2PAddress: destination,
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.Repo.SaveContact(a.Ctx, contact); err != nil {
		return nil, err
	}

	// Отправляем handshake для установления связи
	if a.Messenger != nil {
		go a.Messenger.SendHandshake(destination)
	}

	a.Emitter.Emit("contact_updated")

	return &ContactInfo{
		ID:         contact.ID,
		Nickname:   contact.Nickname,
		I2PAddress: contact.I2PAddress,
		ChatID:     contact.ChatID,
	}, nil
}

// AddContactFromClipboard добавляет контакт из буфера обмена.
func (a *AppCore) AddContactFromClipboard(name string) (*ContactInfo, error) {
	data, err := a.Platform.ClipboardGet()
	if err != nil {
		return nil, fmt.Errorf("clipboard error: %w", err)
	}
	if data == "" {
		return nil, fmt.Errorf("clipboard is empty")
	}
	return a.AddContact(name, data)
}

// DeleteContact удаляет контакт.
func (a *AppCore) DeleteContact(id string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	err := a.Repo.DeleteContact(a.Ctx, id)
	if err == nil {
		a.Emitter.Emit("contact_updated")
	}
	return err
}
