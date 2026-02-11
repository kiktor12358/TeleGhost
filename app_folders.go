package main

// CreateFolder создаёт новую папку.
func (a *App) CreateFolder(name, icon string) error {
	return a.core.CreateFolder(name, icon)
}

// GetFolders возвращает список папок.
func (a *App) GetFolders() ([]*FolderInfo, error) {
	coreFolders, err := a.core.GetFolders()
	if err != nil {
		return nil, err
	}

	result := make([]*FolderInfo, len(coreFolders))
	for i, f := range coreFolders {
		result[i] = &FolderInfo{
			ID:          f.ID,
			Name:        f.Name,
			Icon:        f.Icon,
			ChatIDs:     f.ChatIDs,
			Position:    f.Position,
			UnreadCount: f.UnreadCount,
		}
	}
	return result, nil
}

// UpdateFolder обновляет папку.
func (a *App) UpdateFolder(id, name, icon string) error {
	return a.core.UpdateFolder(id, name, icon)
}

// DeleteFolder удаляет папку.
func (a *App) DeleteFolder(id string) error {
	return a.core.DeleteFolder(id)
}

// AddChatToFolder добавляет чат в папку.
func (a *App) AddChatToFolder(folderID, chatID string) error {
	return a.core.AddChatToFolder(folderID, chatID)
}

// RemoveChatFromFolder удаляет чат из папки.
func (a *App) RemoveChatFromFolder(folderID, chatID string) error {
	return a.core.RemoveChatFromFolder(folderID, chatID)
}
