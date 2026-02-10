package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
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

	// Генерируем временный ChatID на основе адреса, пока не получим публичный ключ
	hasher := sha256.New()
	hasher.Write([]byte(destination))
	chatID := base64.RawURLEncoding.EncodeToString(hasher.Sum(nil)[:16])

	contact := &core.Contact{
		ID:         uuid.New().String(),
		Nickname:   "New Contact",
		I2PAddress: destination,
		ChatID:     chatID,
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	log.Println("[App] AddContactFromClipboard: calling SaveContact")
	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		log.Printf("[App] AddContactFromClipboard: SaveContact error: %v", err)
		return nil, fmt.Errorf("failed to save contact: %w", err)
	}
	log.Println("[App] AddContactFromClipboard: SaveContact success")
	runtime.EventsEmit(a.ctx, "contact_updated")

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

	hasher := sha256.New()
	hasher.Write([]byte(destination))
	chatID := base64.RawURLEncoding.EncodeToString(hasher.Sum(nil)[:16])

	contact := &core.Contact{
		ID:         uuid.New().String(),
		Nickname:   name,
		I2PAddress: destination,
		ChatID:     chatID,
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	log.Println("[App] AddContact: calling SaveContact")
	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		log.Printf("[App] AddContact: SaveContact error: %v", err)
		return nil, fmt.Errorf("failed to save contact: %w", err)
	}
	log.Println("[App] AddContact: SaveContact success")
	runtime.EventsEmit(a.ctx, "contact_updated")

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
	if a.repo == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	log.Println("[App] GetContacts called")
	contacts, err := a.repo.ListContactsWithLastMessage(a.ctx)
	if err != nil {
		log.Printf("[App] GetContacts failed: %v", err)
		return nil, err
	}
	log.Printf("[App] GetContacts found %d contacts", len(contacts))

	result := make([]*ContactInfo, len(contacts))
	for i, c := range contacts {
		lastSeen := ""
		if c.LastSeen != nil {
			lastSeen = c.LastSeen.Format("15:04")
		}

		result[i] = &ContactInfo{
			ID:          c.ID,
			Nickname:    c.Nickname,
			Avatar:      c.Avatar,
			PublicKey:   c.PublicKey,
			I2PAddress:  c.I2PAddress,
			LastMessage: c.LastMessage,
			LastSeen:    lastSeen,
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

	// Если не нашли по ключу, проверяем по адресу (мог быть добавлен вручную без ключа)
	existingContact, err = a.repo.GetContactByAddress(a.ctx, i2pAddress)
	if err == nil && existingContact != nil {
		// Обновляем ключ и ChatID у существующего контакта
		existingContact.PublicKey = pubKey
		existingContact.Nickname = nickname // Можно обновить ник на тот, что прислали
		existingContact.ChatID = identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, pubKey)
		existingContact.UpdatedAt = time.Now()
		a.repo.SaveContact(a.ctx, existingContact)
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
		"ID":         contact.ID,
		"Nickname":   nickname,
		"PublicKey":  pubKey,
		"I2PAddress": i2pAddress[:min(32, len(i2pAddress))] + "...",
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

func (a *App) GetFolders() ([]FolderInfo, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	log.Println("[App] GetFolders called")
	folders, err := a.repo.GetFolders(a.ctx)
	if err != nil {
		log.Printf("[App] GetFolders error: %v", err)
		return nil, err
	}
	log.Printf("[App] GetFolders found %d folders", len(folders))

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
