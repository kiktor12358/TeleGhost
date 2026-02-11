package appcore

import (
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
	pb "teleghost/internal/proto"
	"teleghost/internal/utils"

	"github.com/google/uuid"
)

// SendText отправляет текстовое сообщение.
func (a *AppCore) SendText(contactID, text, replyToID string) error {
	isSelf := a.Identity != nil && contactID == a.Identity.Keys.UserID
	if a.Messenger == nil && !isSelf {
		return fmt.Errorf("not connected to I2P")
	}
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}

	var contact *core.Contact
	if contactID == a.Identity.Keys.UserID {
		contact = &core.Contact{
			ID:        a.Identity.Keys.UserID,
			Nickname:  "Избранное",
			PublicKey: a.Identity.Keys.PublicKeyBase64,
			ChatID:    a.Identity.Keys.UserID, // Use UserID as ChatID for self
		}
	} else {
		var err error
		contact, err = a.Repo.GetContact(a.Ctx, contactID)
		if err != nil || contact == nil {
			return fmt.Errorf("contact not found")
		}

		if contact.ChatID == "" {
			contact.ChatID = identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, contact.PublicKey)
			a.Repo.SaveContact(a.Ctx, contact)
		}
	}

	log.Printf("[AppCore] Sending message to %s (ChatID: %s)", contact.Nickname, contact.ChatID)

	// Handshake если нет публичного ключа
	if contact.PublicKey == "" {
		log.Printf("[AppCore] No public key for %s, sending handshake first...", contact.Nickname)
		if err := a.Messenger.SendHandshake(contact.I2PAddress); err != nil {
			log.Printf("[AppCore] Handshake failed: %v", err)
		}
	}

	if contactID != a.Identity.Keys.UserID {
		if err := a.Messenger.SendTextMessage(contact.I2PAddress, contact.ChatID, text, replyToID); err != nil {
			log.Printf("[AppCore] SendTextMessage error to %s: %v", contact.Nickname, err)
			return fmt.Errorf("send failed: %w", err)
		}
	}

	msg := &core.Message{
		ID:          uuid.New().String(),
		ChatID:      contact.ChatID,
		SenderID:    a.Identity.Keys.UserID,
		Content:     text,
		ContentType: "text",
		Status:      core.MessageStatusSent,
		IsOutgoing:  true,
		Timestamp:   time.Now().UnixMilli(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if replyToID != "" {
		msg.ReplyToID = &replyToID
	}

	a.Repo.SaveMessage(a.Ctx, msg)

	a.Emitter.Emit("new_message", map[string]interface{}{
		"ID":           msg.ID,
		"ChatID":       msg.ChatID,
		"SenderID":     msg.SenderID,
		"Content":      msg.Content,
		"Timestamp":    msg.Timestamp,
		"IsOutgoing":   msg.IsOutgoing,
		"Status":       "sent",
		"ReplyToID":    replyToID,
		"ReplyPreview": a.getReplyPreview(replyToID, contact),
	})

	return nil
}

// GetMessages возвращает историю сообщений.
func (a *AppCore) GetMessages(contactID string, limit, offset int) ([]*MessageInfo, error) {
	if a.Repo == nil {
		return []*MessageInfo{}, nil
	}

	var chatID string
	if contactID == a.Identity.Keys.UserID {
		chatID = a.Identity.Keys.UserID
	} else {
		contact, err := a.Repo.GetContact(a.Ctx, contactID)
		if err != nil || contact == nil {
			return nil, fmt.Errorf("contact not found")
		}
		chatID = contact.ChatID
	}

	messages, err := a.Repo.GetChatHistory(a.Ctx, chatID, limit, offset)
	if err != nil {
		return nil, err
	}

	// Получаем контакты для имен авторов
	contacts, _ := a.Repo.ListContacts(a.Ctx)
	cidToName := make(map[string]string)
	for _, c := range contacts {
		cidToName[c.ID] = c.Nickname
		cidToName[c.PublicKey] = c.Nickname
	}
	cidToName[a.Identity.Keys.UserID] = "Я"

	// Создаем карту всех сообщений для быстрого поиска предпросмотра ответа
	allMsgsMap := make(map[string]*core.Message)
	for _, m := range messages {
		allMsgsMap[m.ID] = m
	}

	result := make([]*MessageInfo, len(messages))
	for i, m := range messages {
		info := &MessageInfo{
			ID:          m.ID,
			Content:     m.Content,
			Timestamp:   m.Timestamp,
			IsOutgoing:  m.IsOutgoing,
			Status:      m.Status.String(),
			ContentType: m.ContentType,
		}

		if m.ReplyToID != nil && *m.ReplyToID != "" {
			info.ReplyToID = *m.ReplyToID
			// Пытаемся найти исходное сообщение для превью
			orig, ok := allMsgsMap[*m.ReplyToID]
			if !ok {
				// Если в текущей пачке нет, ищем в БД
				orig, _ = a.Repo.GetMessage(a.Ctx, *m.ReplyToID)
			}

			if orig != nil {
				author := cidToName[orig.SenderID]
				if author == "" {
					author = "Контакт"
				}
				info.ReplyPreview = &ReplyPreview{
					AuthorName: author,
					Content:    orig.Content,
				}
			}
		}

		if len(m.Attachments) > 0 {
			info.Attachments = make([]map[string]interface{}, len(m.Attachments))
			for j, att := range m.Attachments {
				info.Attachments[j] = map[string]interface{}{
					"ID":           att.ID,
					"Filename":     att.Filename,
					"Size":         att.Size,
					"LocalPath":    att.LocalPath,
					"MimeType":     att.MimeType,
					"IsCompressed": att.IsCompressed,
				}
			}
		}
		result[i] = info
	}

	return result, nil
}

// EditMessage редактирует сообщение.
func (a *AppCore) EditMessage(messageID, newContent string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	return a.Repo.UpdateMessageContent(a.Ctx, messageID, newContent)
}

// DeleteMessage удаляет сообщение.
func (a *AppCore) DeleteMessage(messageID string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	return a.Repo.DeleteMessage(a.Ctx, messageID)
}

// MarkChatAsRead помечает все сообщения в чате как прочитанные.
func (a *AppCore) MarkChatAsRead(chatID string) error {
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}
	return a.Repo.MarkChatAsRead(a.Ctx, chatID)
}

// GetUnreadCount возвращает количество непрочитанных сообщений.
func (a *AppCore) GetUnreadCount() (int, error) {
	if a.Repo == nil {
		return 0, nil
	}
	return a.Repo.GetUnreadCount(a.Ctx)
}

// SendFileMessage отправляет предложение о передаче файлов
func (a *AppCore) SendFileMessage(chatID, text, replyToID string, files []string, isRaw bool) error {
	if a.Messenger == nil {
		return fmt.Errorf("messenger not started")
	}

	var destination string
	var actualChatID string
	var isSelf bool

	var contact *core.Contact
	if chatID == a.Identity.Keys.UserID {
		destination = ""
		actualChatID = a.Identity.Keys.UserID
		isSelf = true
	} else {
		var err error
		contact, err = a.Repo.GetContact(a.Ctx, chatID)
		if err != nil || contact == nil {
			return fmt.Errorf("contact not found")
		}

		destination = contact.I2PAddress
		if contact.ChatID == "" {
			contact.ChatID = identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, contact.PublicKey)
			a.Repo.SaveContact(a.Ctx, contact)
		}
		actualChatID = contact.ChatID

		if contact.PublicKey == "" {
			log.Printf("[AppCore] No public key for %s, sending handshake first...", contact.Nickname)
			if err := a.Messenger.SendHandshake(contact.I2PAddress); err != nil {
				log.Printf("[AppCore] Handshake failed: %v", err)
			}
		}
	}

	now := time.Now().UnixMilli()
	msgID := fmt.Sprintf("%d-%s", now, a.Identity.Keys.UserID[:8])

	// Проверяем, являются ли все файлы изображениями
	allImages := true
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			allImages = false
			break
		}
	}

	if !isRaw && allImages {
		attachments := make([]*pb.Attachment, 0, len(files))
		for _, filePath := range files {
			data, mimeType, width, height, err := utils.CompressImage(filePath, 1280, 1280)
			if err != nil {
				continue
			}

			att := &pb.Attachment{
				Id:           uuid.New().String(),
				Filename:     filepath.Base(filePath),
				MimeType:     mimeType,
				Size:         int64(len(data)),
				Data:         data,
				IsCompressed: true,
				Width:        int32(width),
				Height:       int32(height),
			}
			attachments = append(attachments, att)
		}

		if len(attachments) == 0 {
			return fmt.Errorf("failed to compress any images")
		}

		if !isSelf {
			if err := a.Messenger.SendAttachmentMessageWithID(destination, actualChatID, msgID, text, replyToID, attachments); err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
		}

		coreAttachments := make([]*core.Attachment, 0, len(attachments))
		for _, att := range attachments {
			savedPath, _ := a.SaveAttachment(att.Filename, att.Data)
			coreAtt := &core.Attachment{
				ID:           att.Id,
				Filename:     att.Filename,
				MimeType:     att.MimeType,
				Size:         att.Size,
				LocalPath:    savedPath,
				IsCompressed: att.IsCompressed,
				Width:        int(att.Width),
				Height:       int(att.Height),
			}
			coreAttachments = append(coreAttachments, coreAtt)
		}

		msg := &core.Message{
			ID:          msgID,
			ChatID:      actualChatID,
			SenderID:    a.Identity.Keys.UserID,
			Content:     text,
			ContentType: "mixed",
			Status:      core.MessageStatusSent,
			IsOutgoing:  true,
			Timestamp:   now,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Attachments: coreAttachments,
		}

		if replyToID != "" {
			msg.ReplyToID = &replyToID
		}
		a.Repo.SaveMessage(a.Ctx, msg)

		// Формируем вложения для фронтенда
		infoAttachments := make([]map[string]interface{}, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			infoAttachments = append(infoAttachments, map[string]interface{}{
				"ID":           att.ID,
				"Filename":     att.Filename,
				"Size":         att.Size,
				"LocalPath":    att.LocalPath,
				"MimeType":     att.MimeType,
				"IsCompressed": att.IsCompressed,
				"Width":        att.Width,
				"Height":       att.Height,
			})
		}

		a.Emitter.Emit("new_message", map[string]interface{}{
			"ID":           msg.ID,
			"ChatID":       msg.ChatID,
			"SenderID":     msg.SenderID,
			"Content":      msg.Content,
			"Timestamp":    msg.Timestamp,
			"IsOutgoing":   msg.IsOutgoing,
			"ContentType":  msg.ContentType,
			"Status":       msg.Status.String(),
			"ReplyToID":    replyToID,
			"ReplyPreview": a.getReplyPreview(replyToID, contact),
			"Attachments":  infoAttachments,
		})

		return nil
	}

	// Offer Flow
	a.TransferMu.Lock()
	a.PendingTransfers[msgID] = &PendingTransfer{
		Destination: destination,
		ChatID:      actualChatID,
		Files:       files,
		MessageID:   msgID,
		Timestamp:   now,
	}
	a.TransferMu.Unlock()

	var totalSize int64
	filenames := make([]string, len(files))
	for i, f := range files {
		info, _ := os.Stat(f)
		if info != nil {
			totalSize += info.Size()
		}
		filenames[i] = filepath.Base(f)
	}

	if !isSelf {
		if err := a.Messenger.SendFileOffer(destination, actualChatID, msgID, filenames, totalSize, int32(len(files))); err != nil {
			return fmt.Errorf("failed to send file offer: %w", err)
		}
	}

	msg := &core.Message{
		ID:          msgID,
		ChatID:      actualChatID,
		SenderID:    a.Identity.Keys.UserID,
		Content:     text,
		ContentType: "file_offer",
		Status:      core.MessageStatusSent,
		IsOutgoing:  true,
		Timestamp:   now,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if replyToID != "" {
		msg.ReplyToID = &replyToID
	}

	coreAttachments := make([]*core.Attachment, 0, len(files))
	for _, f := range files {
		stat, _ := os.Stat(f)
		size := int64(0)
		if stat != nil {
			size = stat.Size()
		}
		coreAtt := &core.Attachment{
			ID:           uuid.New().String(),
			MessageID:    msgID,
			Filename:     filepath.Base(f),
			MimeType:     "application/octet-stream",
			Size:         size,
			LocalPath:    f,
			IsCompressed: false,
		}
		coreAttachments = append(coreAttachments, coreAtt)
	}
	msg.Attachments = coreAttachments
	a.Repo.SaveMessage(a.Ctx, msg)

	a.Emitter.Emit("new_message", map[string]interface{}{
		"ID":           msg.ID,
		"ChatID":       msg.ChatID,
		"SenderID":     msg.SenderID,
		"Content":      msg.Content,
		"Timestamp":    msg.Timestamp,
		"IsOutgoing":   msg.IsOutgoing,
		"ContentType":  "file_offer",
		"FileCount":    len(files),
		"TotalSize":    totalSize,
		"Filenames":    filenames,
		"ReplyToID":    replyToID,
		"ReplyPreview": a.getReplyPreview(replyToID, contact),
	})

	return nil
}

// SaveAttachment сохраняет вложение на диск
func (a *AppCore) SaveAttachment(filename string, data []byte) (string, error) {
	if a.Identity == nil {
		return "", fmt.Errorf("user not logged in")
	}

	mediaDir := filepath.Join(a.DataDir, "users", a.Identity.Keys.UserID, "media")
	os.MkdirAll(mediaDir, 0700)

	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	newFilename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), uuid.New().String()[:8], ext)
	fullPath := filepath.Join(mediaDir, newFilename)

	if err := os.WriteFile(fullPath, data, 0600); err != nil {
		return "", err
	}

	return fullPath, nil
}

// AcceptFileTransfer accepts an incoming file offer
func (a *AppCore) AcceptFileTransfer(messageID string) error {
	a.TransferMu.RLock()
	transfer, exists := a.PendingTransfers[messageID]
	a.TransferMu.RUnlock()

	if !exists {
		return fmt.Errorf("transfer not found or expired")
	}

	return a.Messenger.SendFileResponse(transfer.Destination, transfer.ChatID, messageID, true)
}

// DeclineFileTransfer declines an incoming file offer
func (a *AppCore) DeclineFileTransfer(messageID string) error {
	a.TransferMu.RLock()
	transfer, exists := a.PendingTransfers[messageID]
	a.TransferMu.RUnlock()

	if !exists {
		return fmt.Errorf("transfer not found or expired")
	}

	a.Messenger.SendFileResponse(transfer.Destination, transfer.ChatID, messageID, false)

	a.TransferMu.Lock()
	delete(a.PendingTransfers, messageID)
	a.TransferMu.Unlock()

	return nil
}

// onFileOffer handles incoming file transfer offers
func (a *AppCore) onFileOffer(senderPubKey, messageID, chatID string, filenames []string, totalSize int64, fileCount int32) {
	if a.Repo == nil {
		return
	}

	contact, _ := a.Repo.GetContactByPublicKey(a.Ctx, senderPubKey)
	if contact == nil {
		return
	}

	a.TransferMu.Lock()
	a.PendingTransfers[messageID] = &PendingTransfer{
		Destination: contact.I2PAddress,
		ChatID:      contact.ChatID,
		Files:       filenames,
		MessageID:   messageID,
		Timestamp:   time.Now().UnixMilli(),
	}
	a.TransferMu.Unlock()

	msg := &core.Message{
		ID:          messageID,
		ChatID:      contact.ChatID,
		SenderID:    senderPubKey,
		ContentType: "file_offer",
		Status:      core.MessageStatusDelivered,
		IsOutgoing:  false,
		Timestamp:   time.Now().UnixMilli(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	a.Repo.SaveMessage(a.Ctx, msg)

	a.Emitter.Emit("new_message", map[string]interface{}{
		"ID":          msg.ID,
		"ChatID":      msg.ChatID,
		"SenderID":    msg.SenderID,
		"Content":     "Отправлено файлов: " + fmt.Sprint(fileCount),
		"Timestamp":   msg.Timestamp,
		"IsOutgoing":  false,
		"ContentType": "file_offer",
		"TotalSize":   totalSize,
		"FileCount":   fileCount,
	})
}

// onFileResponse handles response to our file offer
func (a *AppCore) onFileResponse(senderPubKey, messageID, chatID string, accepted bool) {
	a.TransferMu.Lock()
	transfer, exists := a.PendingTransfers[messageID]
	a.TransferMu.Unlock()

	if !exists {
		return
	}

	if accepted {
		attachments := make([]*pb.Attachment, 0, len(transfer.Files))
		for _, filePath := range transfer.Files {
			data, _ := os.ReadFile(filePath)
			mimeType := mime.TypeByExtension(filepath.Ext(filePath))
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			att := &pb.Attachment{
				Id:           uuid.New().String(),
				Filename:     filepath.Base(filePath),
				MimeType:     mimeType,
				Size:         int64(len(data)),
				Data:         data,
				IsCompressed: false,
			}
			attachments = append(attachments, att)
		}

		a.Messenger.SendAttachmentMessageWithID(transfer.Destination, transfer.ChatID, messageID, "", "", attachments)
	}

	a.TransferMu.Lock()
	delete(a.PendingTransfers, messageID)
	a.TransferMu.Unlock()
}
