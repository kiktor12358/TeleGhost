package main

import (
	"fmt"
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
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SendFileMessage отправляет сообщение с файлами
func (a *App) SendFileMessage(chatID, text string, files []string, isRaw bool) error {
	if a.messenger == nil {
		return fmt.Errorf("messenger not started")
	}

	contact, err := a.repo.GetContact(a.ctx, chatID)
	if err != nil || contact == nil {
		return fmt.Errorf("contact not found")
	}

	destination := contact.I2PAddress
	actualChatID := contact.ChatID
	if actualChatID == "" {
		actualChatID = identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, contact.PublicKey)
	}

	if isRaw {
		now := time.Now().UnixMilli()
		msgID := fmt.Sprintf("%d-%s", now, a.identity.Keys.UserID[:8])

		a.transferMu.Lock()
		a.pendingTransfers[msgID] = &PendingTransfer{
			Destination: destination,
			ChatID:      actualChatID,
			Files:       files,
			MessageID:   msgID,
			Timestamp:   now,
		}
		a.transferMu.Unlock()

		var totalSize int64
		filenames := make([]string, len(files))
		for i, f := range files {
			info, _ := os.Stat(f)
			if info != nil {
				totalSize += info.Size()
			}
			filenames[i] = filepath.Base(f)
		}

		if err := a.messenger.SendFileOffer(destination, actualChatID, msgID, filenames, totalSize, int32(len(files))); err != nil {
			return fmt.Errorf("failed to send file offer: %w", err)
		}

		msg := &core.Message{
			ID:          msgID,
			ChatID:      actualChatID,
			SenderID:    a.identity.Keys.PublicKeyBase64,
			Content:     text,
			ContentType: "file_offer",
			Status:      core.MessageStatusSent,
			IsOutgoing:  true,
			Timestamp:   now,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
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
		a.repo.SaveMessage(a.ctx, msg)

		runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
			"ID":          msg.ID,
			"ChatID":      msg.ChatID,
			"SenderID":    msg.SenderID,
			"Content":     msg.Content,
			"Timestamp":   msg.Timestamp,
			"IsOutgoing":  msg.IsOutgoing,
			"ContentType": "file_offer",
			"FileCount":   len(files),
			"TotalSize":   totalSize,
			"Filenames":   filenames,
		})

		return nil
	}

	attachments := make([]*pb.Attachment, 0, len(files))
	for _, filePath := range files {
		var data []byte
		var mimeType string
		var width, height int
		var isCompressed bool

		ext := strings.ToLower(filepath.Ext(filePath))
		isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"

		if !isImage {
			data, _ = os.ReadFile(filePath)
			mimeType = mime.TypeByExtension(ext)
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			isCompressed = false
		} else {
			data, mimeType, width, height, _ = utils.CompressImage(filePath, 1280, 1280)
			isCompressed = true
		}

		att := &pb.Attachment{
			Id:           uuid.New().String(),
			Filename:     filepath.Base(filePath),
			MimeType:     mimeType,
			Size:         int64(len(data)),
			Data:         data,
			IsCompressed: isCompressed,
			Width:        int32(width),
			Height:       int32(height),
		}
		attachments = append(attachments, att)
	}

	now := time.Now().UnixMilli()
	msgID := fmt.Sprintf("%d-%s", now, a.identity.Keys.UserID[:8])

	if err := a.messenger.SendAttachmentMessageWithID(destination, actualChatID, msgID, text, attachments); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	coreAttachments := make([]*core.Attachment, 0, len(attachments))
	for _, att := range attachments {
		savedPath, _ := a.saveAttachment(att.Filename, att.Data)
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
		SenderID:    a.identity.Keys.PublicKeyBase64,
		Content:     text,
		ContentType: "mixed",
		Status:      core.MessageStatusSent,
		IsOutgoing:  true,
		Timestamp:   now,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Attachments: coreAttachments,
	}
	a.repo.SaveMessage(a.ctx, msg)

	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"ID":          msg.ID,
		"ChatID":      msg.ChatID,
		"SenderID":    msg.SenderID,
		"Content":     msg.Content,
		"Timestamp":   msg.Timestamp,
		"IsOutgoing":  msg.IsOutgoing,
		"ContentType": msg.ContentType,
		"Status":      msg.Status.String(),
	})

	return nil
}

// saveAttachment сохраняет вложение на диск
func (a *App) saveAttachment(filename string, data []byte) (string, error) {
	if a.identity == nil {
		return "", fmt.Errorf("user not logged in")
	}

	mediaDir := filepath.Join(a.dataDir, "users", a.identity.Keys.UserID, "media")
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

// onMessageReceived обработчик входящих сообщений
func (a *App) onMessageReceived(msg *core.Message, senderPubKey, senderAddr string) {
	if a.repo == nil {
		return
	}

	contact, _ := a.repo.GetContactByPublicKey(a.ctx, senderPubKey)
	if contact == nil {
		contact, _ = a.repo.GetContactByAddress(a.ctx, senderAddr)
		if contact != nil {
			contact.PublicKey = senderPubKey
			newChatID := identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, senderPubKey)
			contact.ChatID = newChatID
			a.repo.SaveContact(a.ctx, contact)
		}
	}

	if contact != nil {
		msg.ChatID = contact.ChatID
	}

	if err := a.repo.SaveMessage(a.ctx, msg); err != nil {
		return
	}

	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"ID":          msg.ID,
		"ChatID":      msg.ChatID,
		"SenderID":    msg.SenderID,
		"Content":     msg.Content,
		"Timestamp":   msg.Timestamp,
		"IsOutgoing":  msg.IsOutgoing,
		"ContentType": msg.ContentType,
	})
}

// SendText отправляет текстовое сообщение
func (a *App) SendText(contactID, text string) error {
	if a.messenger == nil {
		return fmt.Errorf("not connected to I2P")
	}

	contact, err := a.repo.GetContact(a.ctx, contactID)
	if err != nil || contact == nil {
		return fmt.Errorf("contact not found")
	}

	if contact.ChatID == "" {
		contact.ChatID = identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, contact.PublicKey)
		a.repo.SaveContact(a.ctx, contact)
	}

	a.messenger.SendHandshake(contact.I2PAddress)

	if err := a.messenger.SendTextMessage(contact.I2PAddress, contact.ChatID, text); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	msg := &core.Message{
		ID:          uuid.New().String(),
		ChatID:      contact.ChatID,
		SenderID:    a.identity.Keys.UserID,
		Content:     text,
		ContentType: "text",
		Status:      core.MessageStatusSent,
		IsOutgoing:  true,
		Timestamp:   time.Now().UnixMilli(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	a.repo.SaveMessage(a.ctx, msg)

	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"ID":         msg.ID,
		"ChatID":     msg.ChatID,
		"SenderID":   msg.SenderID,
		"Content":    msg.Content,
		"Timestamp":  msg.Timestamp,
		"IsOutgoing": msg.IsOutgoing,
		"Status":     "sent",
	})

	return nil
}

// GetMessages возвращает сообщения чата с контактом
func (a *App) GetMessages(contactID string, limit, offset int) ([]*MessageInfo, error) {
	contact, err := a.repo.GetContact(a.ctx, contactID)
	if err != nil || contact == nil {
		return nil, fmt.Errorf("contact not found")
	}

	messages, err := a.repo.GetChatHistory(a.ctx, contact.ChatID, limit, offset)
	if err != nil {
		return nil, err
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

// EditMessage редактирует содержимое сообщения
func (a *App) EditMessage(messageID, newContent string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.UpdateMessageContent(a.ctx, messageID, newContent)
}

// DeleteMessage удаляет сообщение локально
func (a *App) DeleteMessage(messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.DeleteMessage(a.ctx, messageID)
}

// DeleteMessageForAll удаляет сообщение у всех участников
func (a *App) DeleteMessageForAll(messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.repo.DeleteMessage(a.ctx, messageID)
}

// onFileOffer handles incoming file transfer offers
func (a *App) onFileOffer(senderPubKey, messageID, chatID string, filenames []string, totalSize int64, fileCount int32) {
	if a.repo == nil {
		return
	}

	contact, _ := a.repo.GetContactByPublicKey(a.ctx, senderPubKey)
	if contact == nil {
		return
	}

	a.transferMu.Lock()
	a.pendingTransfers[messageID] = &PendingTransfer{
		Destination: contact.I2PAddress,
		ChatID:      contact.ChatID,
		Files:       filenames,
		MessageID:   messageID,
		Timestamp:   time.Now().UnixMilli(),
	}
	a.transferMu.Unlock()

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
	a.repo.SaveMessage(a.ctx, msg)

	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
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
func (a *App) onFileResponse(senderPubKey, messageID, chatID string, accepted bool) {
	a.transferMu.Lock()
	transfer, exists := a.pendingTransfers[messageID]
	a.transferMu.Unlock()

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

		a.messenger.SendAttachmentMessageWithID(transfer.Destination, transfer.ChatID, messageID, "", attachments)
	}

	a.transferMu.Lock()
	delete(a.pendingTransfers, messageID)
	a.transferMu.Unlock()
}

// AcceptFileTransfer accepts an incoming file offer
func (a *App) AcceptFileTransfer(messageID string) error {
	a.transferMu.RLock()
	transfer, exists := a.pendingTransfers[messageID]
	a.transferMu.RUnlock()

	if !exists {
		return fmt.Errorf("transfer not found or expired")
	}

	return a.messenger.SendFileResponse(transfer.Destination, transfer.ChatID, messageID, true)
}

// DeclineFileTransfer declines an incoming file offer
func (a *App) DeclineFileTransfer(messageID string) error {
	a.transferMu.RLock()
	transfer, exists := a.pendingTransfers[messageID]
	a.transferMu.RUnlock()

	if !exists {
		return fmt.Errorf("transfer not found or expired")
	}

	a.messenger.SendFileResponse(transfer.Destination, transfer.ChatID, messageID, false)

	a.transferMu.Lock()
	delete(a.pendingTransfers, messageID)
	a.transferMu.Unlock()

	return nil
}
