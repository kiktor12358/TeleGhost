package main

import (
	"fmt"
	"strings"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AddContactFromClipboard добавляет контакт из буфера обмена
func (a *App) AddContactFromClipboard() (*ContactInfo, error) {
	data, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("clipboard is empty")
	}

	destination := strings.TrimSpace(data)
	if len(destination) < 50 {
		return nil, fmt.Errorf("invalid I2P destination (too short)")
	}

	contact := &core.Contact{
		ID:         uuid.New().String(),
		Nickname:   "New Contact",
		I2PAddress: destination,
		ChatID:     uuid.New().String(),
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		return nil, fmt.Errorf("failed to save contact: %w", err)
	}

	return &ContactInfo{
		ID:         contact.ID,
		Nickname:   contact.Nickname,
		I2PAddress: destination[:32] + "...",
		ChatID:     contact.ChatID,
	}, nil
}

// AddContact добавляет контакт по I2P адресу
func (a *App) AddContact(name, destination string) (*ContactInfo, error) {
	destination = strings.TrimSpace(destination)
	if len(destination) < 50 {
		return nil, fmt.Errorf("invalid I2P destination (too short)")
	}

	contact := &core.Contact{
		ID:         uuid.New().String(),
		Nickname:   name,
		I2PAddress: destination,
		ChatID:     uuid.New().String(),
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		return nil, fmt.Errorf("failed to save contact: %w", err)
	}

	return &ContactInfo{
		ID:         contact.ID,
		Nickname:   contact.Nickname,
		I2PAddress: destination[:32] + "...",
		ChatID:     contact.ChatID,
	}, nil
}

// DeleteContact удаляет контакт
func (a *App) DeleteContact(id string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.DeleteContact(a.ctx, id)
}

// GetContacts возвращает список контактов
func (a *App) GetContacts() ([]*ContactInfo, error) {
	contacts, err := a.repo.ListContacts(a.ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*ContactInfo, len(contacts))
	for i, c := range contacts {
		lastMsg := ""
		messages, _ := a.repo.GetChatHistory(a.ctx, c.ChatID, 1, 0)
		if len(messages) > 0 {
			lastMsg = messages[0].Content
			if len(lastMsg) > 30 {
				lastMsg = lastMsg[:30] + "..."
			}
		}

		result[i] = &ContactInfo{
			ID:          c.ID,
			Nickname:    c.Nickname,
			Avatar:      c.Avatar,
			PublicKey:   c.PublicKey,
			I2PAddress:  c.I2PAddress,
			LastMessage: lastMsg,
			LastSeen:    c.LastSeen.Format("15:04"),
			ChatID:      c.ChatID,
		}
	}

	return result, nil
}

// onContactRequest обработчик входящих handshake
func (a *App) onContactRequest(pubKey, nickname, i2pAddress string) {
	if a.repo == nil || a.identity == nil {
		return
	}

	existingContact, err := a.repo.GetContactByPublicKey(a.ctx, pubKey)
	if err == nil && existingContact != nil {
		if existingContact.I2PAddress != i2pAddress {
			existingContact.I2PAddress = i2pAddress
			existingContact.UpdatedAt = time.Now()
			a.repo.SaveContact(a.ctx, existingContact)
		}
		return
	}

	chatID := identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, pubKey)
	contact := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  pubKey,
		Nickname:   nickname,
		I2PAddress: i2pAddress,
		ChatID:     chatID,
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	a.repo.SaveContact(a.ctx, contact)

	runtime.EventsEmit(a.ctx, "new_contact", map[string]interface{}{
		"id":         contact.ID,
		"nickname":   nickname,
		"publicKey":  pubKey,
		"i2pAddress": i2pAddress[:min(32, len(i2pAddress))] + "...",
	})
}

// === Folder API ===

// CreateFolder создаёт новую папку
func (a *App) CreateFolder(name, icon string) (string, error) {
	if a.repo == nil {
		return "", fmt.Errorf("database not initialized")
	}

	id := uuid.New().String()
	folders, _ := a.repo.GetFolders(a.ctx)
	position := len(folders)

	folder := &core.Folder{
		ID:       id,
		Name:     name,
		Icon:     icon,
		Position: position,
	}

	if err := a.repo.CreateFolder(a.ctx, folder); err != nil {
		return "", err
	}

	return id, nil
}

// GetFolders возвращает все папки с их чатами
func (a *App) GetFolders() ([]FolderInfo, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	folders, err := a.repo.GetFolders(a.ctx)
	if err != nil {
		return nil, err
	}

	result := make([]FolderInfo, 0, len(folders))
	for _, f := range folders {
		chatIDs, err := a.repo.GetFolderChats(a.ctx, f.ID)
		if err != nil {
			chatIDs = []string{}
		}

		result = append(result, FolderInfo{
			ID:       f.ID,
			Name:     f.Name,
			Icon:     f.Icon,
			ChatIDs:  chatIDs,
			Position: f.Position,
		})
	}

	return result, nil
}

// DeleteFolder удаляет папку
func (a *App) DeleteFolder(id string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.DeleteFolder(a.ctx, id)
}

// UpdateFolder обновляет данные папки
func (a *App) UpdateFolder(id, name, icon string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}

	folder := &core.Folder{
		ID:   id,
		Name: name,
		Icon: icon,
	}

	return a.repo.CreateFolder(a.ctx, folder)
}

// AddChatToFolder добавляет чат в папку
func (a *App) AddChatToFolder(folderID, contactID string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.AddChatToFolder(a.ctx, folderID, contactID)
}

// RemoveChatFromFolder удаляет чат из папки
func (a *App) RemoveChatFromFolder(folderID, contactID string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.RemoveChatFromFolder(a.ctx, folderID, contactID)
}
