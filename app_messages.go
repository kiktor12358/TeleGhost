package main

// SendText отправляет текстовое сообщение.
func (a *App) SendText(contactID, text, replyToID string) error {
	return a.core.SendText(contactID, text, replyToID)
}

// SendFileMessage отправляет файлы.
func (a *App) SendFileMessage(chatID, text, replyToID string, files []string, isRaw bool) error {
	return a.core.SendFileMessage(chatID, text, replyToID, files, isRaw)
}

// GetMessages возвращает историю сообщений.
func (a *App) GetMessages(contactID string, limit, offset int) ([]*MessageInfo, error) {
	coreMsgs, err := a.core.GetMessages(contactID, limit, offset)
	if err != nil {
		return nil, err
	}

	result := make([]*MessageInfo, len(coreMsgs))
	for i, m := range coreMsgs {
		info := &MessageInfo{
			ID:           m.ID,
			Content:      m.Content,
			Timestamp:    m.Timestamp,
			IsOutgoing:   m.IsOutgoing,
			Status:       m.Status,
			ContentType:  m.ContentType,
			Attachments:  m.Attachments,
			ReplyToID:    m.ReplyToID,
			ReplyPreview: m.ReplyPreview,
		}
		result[i] = info
	}
	return result, nil
}

// EditMessage редактирует сообщение.
func (a *App) EditMessage(messageID, newContent string) error {
	return a.core.EditMessage(messageID, newContent)
}

// DeleteMessage удаляет сообщение.
func (a *App) DeleteMessage(messageID string) error {
	return a.core.DeleteMessage(messageID)
}

// MarkChatAsRead помечает чат прочитанным.
func (a *App) MarkChatAsRead(chatID string) error {
	return a.core.MarkChatAsRead(chatID)
}

// GetUnreadCount возвращает общее количество непрочитанных.
func (a *App) GetUnreadCount() (int, error) {
	return a.core.GetUnreadCount()
}

// AcceptFileTransfer принимает файлы.
func (a *App) AcceptFileTransfer(messageID string) error {
	return a.core.AcceptFileTransfer(messageID)
}

// DeclineFileTransfer отклоняет файлы.
func (a *App) DeclineFileTransfer(messageID string) error {
	return a.core.DeclineFileTransfer(messageID)
}
