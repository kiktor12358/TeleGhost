package appcore

import (
	"fmt"
	"teleghost/internal/core"

	"github.com/google/uuid"
)

// CreateFolder создаёт новую папку.
func (a *AppCore) CreateFolder(name, icon string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	folder := &core.Folder{
		ID:   uuid.New().String(),
		Name: name,
		Icon: icon,
	}
	return a.Repo.CreateFolder(a.Ctx, folder)
}

// GetFolders возвращает все папки с ID чатов.
func (a *AppCore) GetFolders() ([]*FolderInfo, error) {
	if a.Repo == nil {
		return []*FolderInfo{}, nil
	}

	folders, err := a.Repo.GetFolders(a.Ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*FolderInfo, len(folders))
	for i, f := range folders {
		chatIDs, _ := a.Repo.GetFolderChats(a.Ctx, f.ID)

		// Считаем непрочитанные для папки
		unreadTotal := 0
		unreadMap, _ := a.Repo.GetUnreadCountByChat(a.Ctx)
		for _, chatID := range chatIDs {
			if count, ok := unreadMap[chatID]; ok {
				unreadTotal += count
			}
		}

		result[i] = &FolderInfo{
			ID:          f.ID,
			Name:        f.Name,
			Icon:        f.Icon,
			Position:    f.Position,
			ChatIDs:     chatIDs,
			UnreadCount: unreadTotal,
		}
	}

	return result, nil
}

// DeleteFolder удаляет папку.
func (a *AppCore) DeleteFolder(id string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	return a.Repo.DeleteFolder(a.Ctx, id)
}

// UpdateFolder обновляет папку.
func (a *AppCore) UpdateFolder(id, name, icon string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	folder := &core.Folder{ID: id, Name: name, Icon: icon}
	return a.Repo.CreateFolder(a.Ctx, folder) // Upsert
}

// AddChatToFolder добавляет чат в папку.
func (a *AppCore) AddChatToFolder(folderID, contactID string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	return a.Repo.AddChatToFolder(a.Ctx, folderID, contactID)
}

// RemoveChatFromFolder удаляет чат из папки.
func (a *AppCore) RemoveChatFromFolder(folderID, contactID string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	return a.Repo.RemoveChatFromFolder(a.Ctx, folderID, contactID)
}
