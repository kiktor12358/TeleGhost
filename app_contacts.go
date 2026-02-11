package main

// GetContacts возвращает список контактов.
func (a *App) GetContacts() ([]*ContactInfo, error) {
	coreContacts, err := a.core.GetContacts()
	if err != nil {
		return nil, err
	}

	// Маппим внутреннюю структуру в структуру фронтенда
	result := make([]*ContactInfo, len(coreContacts))
	for i, c := range coreContacts {
		info := &ContactInfo{
			ID:          c.ID,
			Nickname:    c.Nickname,
			PublicKey:   c.PublicKey,
			Avatar:      c.Avatar,
			I2PAddress:  c.I2PAddress,
			ChatID:      c.ChatID,
			UnreadCount: c.UnreadCount,
		}
		if c.LastMessage != "" {
			info.LastMessage = c.LastMessage
		}
		if c.LastMessageTime != nil {
			info.LastMessageTime = c.LastMessageTime.UnixMilli()
		}
		result[i] = info
	}
	return result, nil
}

// AddContact добавляет контакт.
func (a *App) AddContact(name, dest string) (*ContactInfo, error) {
	c, err := a.core.AddContact(name, dest)
	if err != nil {
		return nil, err
	}
	return &ContactInfo{
		ID:         c.ID,
		Nickname:   c.Nickname,
		I2PAddress: c.I2PAddress,
		ChatID:     c.ChatID,
	}, nil
}

// AddContactFromClipboard добавляет контакт из буфера обмена.
func (a *App) AddContactFromClipboard(name string) (*ContactInfo, error) {
	c, err := a.core.AddContactFromClipboard(name)
	if err != nil {
		return nil, err
	}
	return &ContactInfo{
		ID:         c.ID,
		Nickname:   c.Nickname,
		I2PAddress: c.I2PAddress,
		ChatID:     c.ChatID,
	}, nil
}

// DeleteContact удаляет контакт.
func (a *App) DeleteContact(id string) error {
	return a.core.DeleteContact(id)
}
