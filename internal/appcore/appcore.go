// Package appcore содержит общую бизнес-логику TeleGhost.
//
// Этот пакет используется И десктопной (Wails), И мобильной (HTTP) версией.
// Платформо-специфичные вещи (события, диалоги, буфер обмена) абстрагированы
// через интерфейсы EventEmitter и PlatformServices.
//
// Архитектура:
//
//	main.go (Desktop)  → AppCore + WailsEmitter
//	mobile/mobile.go   → AppCore + SSEEmitter (HTTP)
package appcore

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
	"teleghost/internal/network/messenger"
	"teleghost/internal/network/profiles"
	"teleghost/internal/network/router"
	"teleghost/internal/repository/sqlite"

	"github.com/go-i2p/i2pkeys"

	"github.com/google/uuid"
)

// ─── Интерфейсы платформы ───────────────────────────────────────────────────

// EventEmitter — абстракция для отправки событий во фронтенд.
// Desktop: Wails runtime.EventsEmit
// Mobile: SSE push
type EventEmitter interface {
	Emit(event string, data ...interface{})
}

// PlatformServices — абстракция для платформо-специфичных сервисов.
// Desktop: Wails file dialogs, clipboard, window management
// Mobile: no-op или HTML5 эквиваленты
type PlatformServices interface {
	// OpenFileDialog открывает диалог выбора файла. На мобилке — no-op.
	OpenFileDialog(title string, filters []string) (string, error)
	// SaveFileDialog открывает диалог сохранения файла. На мобилке — no-op.
	SaveFileDialog(title, defaultFilename string) (string, error)
	// ClipboardSet копирует текст в буфер обмена.
	ClipboardSet(text string)
	// ClipboardGet получает текст из буфера обмена.
	ClipboardGet() (string, error)
	// ShowWindow показывает окно приложения.
	ShowWindow()
	// HideWindow скрывает окно приложения.
	HideWindow()
	Notify(title, message string)
}

// ─── Типы данных (для API bridge, совместим со фронтендом) ────────────────────

// NetworkStatus — статус сети
type NetworkStatus string

const (
	StatusOffline    NetworkStatus = "offline"
	StatusConnecting NetworkStatus = "connecting"
	StatusOnline     NetworkStatus = "online"
	StatusError      NetworkStatus = "error"
)

// ReplyPreview содержит краткую информацию об исходном сообщении для ответа
type ReplyPreview struct {
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
}

// FolderInfo — информация о папке (для фронтенда)
type FolderInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Icon        string   `json:"icon"`
	Position    int      `json:"position"`
	ChatIDs     []string `json:"chat_ids"`
	UnreadCount int      `json:"unread_count"`
}

// ContactInfo — информация о контакте (для фронтенда)
type ContactInfo struct {
	ID              string     `json:"ID"`
	Nickname        string     `json:"Nickname"`
	Bio             string     `json:"Bio"`
	Avatar          string     `json:"Avatar"`
	I2PAddress      string     `json:"I2PAddress"`
	PublicKey       string     `json:"PublicKey"`
	ChatID          string     `json:"ChatID"`
	IsBlocked       bool       `json:"IsBlocked"`
	IsVerified      bool       `json:"IsVerified"`
	LastMessage     string     `json:"LastMessage"`
	LastMessageTime *time.Time `json:"LastMessageTime"`
	UnreadCount     int        `json:"UnreadCount"`
}

const (
	// MaxAvatarSize — максимальный размер аватарки (512 КБ)
	MaxAvatarSize = 512 * 1024
)

// SaveAvatar сохраняет аватарку в несжатом/нешифрованном виде
func (a *AppCore) SaveAvatar(filename string, data []byte) (string, error) {
	if a.Identity == nil {
		return "", fmt.Errorf("user not logged in")
	}

	if len(data) > MaxAvatarSize {
		return "", fmt.Errorf("изображение слишком большое (макс. %d байт)", MaxAvatarSize)
	}

	userDir := filepath.Join(a.DataDir, "users", a.Identity.Keys.UserID)
	avatarsDir := filepath.Join(userDir, "avatars")
	os.MkdirAll(avatarsDir, 0755)

	fullPath := filepath.Join(avatarsDir, filename)
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", err
	}

	return fullPath, nil
}

// MessageInfo — информация о сообщении (для фронтенда)
type MessageInfo struct {
	ID           string                   `json:"ID"`
	Content      string                   `json:"Content"`
	Timestamp    int64                    `json:"Timestamp"`
	IsOutgoing   bool                     `json:"IsOutgoing"`
	Status       string                   `json:"Status"`
	ContentType  string                   `json:"ContentType"`
	ReplyToID    string                   `json:"ReplyToID,omitempty"`
	ReplyPreview *ReplyPreview            `json:"ReplyPreview,omitempty"`
	Attachments  []map[string]interface{} `json:"Attachments,omitempty"`
}

// UserInfo — информация о пользователе
type UserInfo struct {
	ID          string `json:"ID"`
	PublicKey   string `json:"PublicKey"`
	Nickname    string `json:"Nickname"`
	Avatar      string `json:"Avatar"`
	Destination string `json:"Destination"`
}

// AppAboutInfo — информация о приложении
type AppAboutInfo struct {
	AppVersion string `json:"app_version"`
	I2PVersion string `json:"i2p_version"`
	I2PPath    string `json:"i2p_path"`
	Author     string `json:"author"`
	License    string `json:"license"`
}

// PendingTransfer — ожидающая файловая передача
type PendingTransfer struct {
	Destination string
	ChatID      string
	Files       []string
	MessageID   string
	Timestamp   int64
}

// RouterSettings — настройки роутера
type RouterSettings struct {
	TunnelLength int  `json:"tunnelLength"`
	LogToFile    bool `json:"logToFile"`
}

// ─── AppCore — единое ядро приложения ───────────────────────────────────────

// AppCore содержит ВСЮ бизнес-логику TeleGhost.
// Используется и десктопной, и мобильной версией.
type AppCore struct {
	Ctx    context.Context
	Cancel context.CancelFunc

	Identity       *identity.Identity
	Repo           *sqlite.Repository
	Router         *router.SAMRouter
	Messenger      *messenger.Service
	ProfileManager *profiles.ProfileManager
	Emitter        EventEmitter
	Platform       PlatformServices

	Status  NetworkStatus
	DataDir string

	IsFocused    bool   // Текущий статус фокуса окна
	IsVisible    bool   // Видимо ли окно (не в трее)
	ActiveChatID string // ID чата, который сейчас открыт

	TransferMu       sync.RWMutex
	PendingTransfers map[string]*PendingTransfer

	mu sync.RWMutex
}

// NewAppCore создаёт новое ядро приложения.
func NewAppCore(dataDir string, emitter EventEmitter, platform PlatformServices) *AppCore {
	ctx, cancel := context.WithCancel(context.Background())

	app := &AppCore{
		Ctx:              ctx,
		Cancel:           cancel,
		DataDir:          dataDir,
		Status:           StatusOffline,
		Emitter:          emitter,
		Platform:         platform,
		IsVisible:        true,
		PendingTransfers: make(map[string]*PendingTransfer),
	}

	return app
}

// Init инициализирует компоненты приложения (директории, профиль-менеджер).
func (a *AppCore) Init() error {
	// Создаём директории
	os.MkdirAll(a.DataDir, 0700)
	os.MkdirAll(filepath.Join(a.DataDir, "users"), 0700)

	// Инициализируем менеджер профилей
	profilesDir := filepath.Join(a.DataDir, "profiles")
	pm, err := profiles.NewProfileManager(profilesDir)
	if err != nil {
		return fmt.Errorf("failed to init profile manager: %w", err)
	}
	a.ProfileManager = pm

	return nil
}

// Shutdown корректно останавливает все компоненты.
func (a *AppCore) Shutdown() {
	log.Println("[AppCore] Shutting down...")

	if a.Messenger != nil {
		a.Messenger.Stop()
	}
	if a.Router != nil {
		a.Router.Stop()
	}
	if a.Repo != nil {
		a.Repo.Close()
	}
	a.Cancel()
}

// ─── Auth Methods ───────────────────────────────────────────────────────────

// UpdateMyProfile обновляет профиль пользователя в БД и в менеджере профилей.
func (a *AppCore) UpdateMyProfile(nickname, bio, avatar string) error {
	log.Printf("[AppCore] Updating profile: nickname=%s, bio=%s, avatarLen=%d", nickname, bio, len(avatar))
	if a.Repo == nil {
		return fmt.Errorf("not logged in")
	}

	// Обновляем в БД
	if err := a.Repo.UpdateMyProfile(a.Ctx, nickname, bio, avatar); err != nil {
		return err
	}

	// Синхронизируем с ProfileManager (чтобы на экране входа были актуальные данные)
	if a.ProfileManager != nil && a.Identity != nil {
		avatarPath := avatar
		// Если avatar - это base64 или пришел новый путь, сохраняем его локально
		if len(avatar) > 30 && (strings.HasPrefix(avatar, "data:image") || strings.HasPrefix(avatar, "image")) {
			// Пытаемся декодировать base64 в байты
			var data []byte
			var err error
			if idx := strings.Index(avatar, ","); idx != -1 {
				data, err = base64.StdEncoding.DecodeString(avatar[idx+1:])
			} else {
				data, err = base64.StdEncoding.DecodeString(avatar)
			}

			if err == nil {
				if len(data) > MaxAvatarSize {
					return fmt.Errorf("аватарка слишком большая (максимум %d КБ)", MaxAvatarSize/1024)
				}
				// Сохраняем байты в файл аватара пользователя (НЕ зашифровано!)
				newPath, err := a.SaveAvatar("my_avatar.png", data)
				if err == nil {
					avatarPath = newPath
					// Также обновляем в БД путь на локальный, а не base64
					a.Repo.UpdateMyProfile(a.Ctx, nickname, bio, avatarPath)
				}
			} else {
				log.Printf("[AppCore] Failed to decode base64 avatar: %v", err)
			}
		}

		if meta, _ := a.ProfileManager.GetProfileByUserID(a.Identity.Keys.UserID); meta != nil {
			a.ProfileManager.UpdateProfile(meta.ID, nickname, avatarPath, false, meta.UsePin, "", a.Identity.Mnemonic)
		}
	}

	return nil
}

// GetCurrentProfile возвращает текущий профиль.
func (a *AppCore) GetCurrentProfile() map[string]interface{} {
	if a.ProfileManager == nil || a.Identity == nil {
		return nil
	}
	profilesList, _ := a.ProfileManager.ListProfiles()
	for _, p := range profilesList {
		if p.UserID == a.Identity.Keys.UserID {
			return map[string]interface{}{
				"id":           p.ID,
				"display_name": p.DisplayName,
				"user_id":      p.UserID,
				"avatar_path":  a.formatProfileAvatarURL(p.UserID, p.AvatarPath),
				"use_pin":      p.UsePin,
			}
		}
	}
	return nil
}

// SetNetworkStatus устанавливает статус сети и уведомляет фронтенд.
func (a *AppCore) SetNetworkStatus(status NetworkStatus) {
	log.Printf("[AppCore] Network status changed: %s", status)
	a.mu.Lock()
	a.Status = status
	a.mu.Unlock()
	a.Emitter.Emit("network_status", string(status))
}

// ─── Utility Methods ────────────────────────────────────────────────────────

// CopyToClipboard копирует текст в буфер обмена.
func (a *AppCore) CopyToClipboard(text string) {
	a.Platform.ClipboardSet(text)
}

// GetFileBase64 читает файл и возвращает base64.
func (a *AppCore) GetFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ─── I2P Network ────────────────────────────────────────────────────────────

// InitUserRepository инициализирует БД пользователя.
func (a *AppCore) InitUserRepository(userID string) error {
	userDir := filepath.Join(a.DataDir, "users", userID)
	os.MkdirAll(userDir, 0700)

	dbPath := filepath.Join(userDir, "data.db")

	var keys *identity.Keys
	if a.Identity != nil {
		keys = a.Identity.Keys
	}

	repo, err := sqlite.New(dbPath, keys)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	a.Repo = repo

	if err := repo.Migrate(a.Ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

// ConnectToI2P подключается к I2P сети.
func (a *AppCore) ConnectToI2P() {
	a.SetNetworkStatus(StatusConnecting)

	routerSettings := a.GetRouterSettings()
	cfg := router.DefaultConfig()
	cfg.InboundLength = routerSettings.TunnelLength
	cfg.OutboundLength = routerSettings.TunnelLength

	a.Router = router.NewSAMRouter(cfg)

	// Загружаем существующие ключи из БД
	if a.Repo != nil {
		user, err := a.Repo.GetMyProfile(a.Ctx)
		if err == nil && user != nil && len(user.I2PKeys) > 0 {
			log.Println("[AppCore] Loading existing I2P keys from database")
			keysPath := filepath.Join(a.DataDir, "users", a.Identity.Keys.UserID, "i2p_keys.dat")
			if err := os.WriteFile(keysPath, user.I2PKeys, 0600); err == nil {
				keys, err := i2pkeys.LoadKeys(keysPath)
				if err == nil {
					a.Router.SetKeys(keys)
				} else {
					log.Printf("[AppCore] Failed to load I2P keys from %s: %v", keysPath, err)
				}
				// Удаляем временный файл ключей после загрузки (опционально, но безопаснее хранить в БД)
				os.Remove(keysPath)
			}
		}
	}

	if err := a.Router.Start(a.Ctx); err != nil {
		if a.Ctx.Err() != nil {
			log.Println("[AppCore] I2P connection canceled")
			return
		}
		a.SetNetworkStatus(StatusError)
		log.Printf("[AppCore] I2P connection failed: %v", err)
		return
	}

	// Если ключи были сгенерированы заново, сохраняем их
	if a.Repo != nil && a.Identity != nil {
		keys := a.Router.GetKeys()
		dest := a.Router.GetDestination()

		user, _ := a.Repo.GetMyProfile(a.Ctx)
		if user != nil {
			keysPath := filepath.Join(a.DataDir, "users", a.Identity.Keys.UserID, "temp_i2p_keys.dat")
			if err := i2pkeys.StoreKeys(keys, keysPath); err == nil {
				if keysData, err := os.ReadFile(keysPath); err == nil {
					user.I2PKeys = keysData
					user.I2PAddress = dest
					log.Printf("[AppCore] Saving I2P destination to DB: %s", dest)
					a.Repo.SaveUser(a.Ctx, user)
				}
				os.Remove(keysPath)
			}
		}
	}

	// Запускаем messenger
	a.Messenger = messenger.NewService(a.Router, a.Identity.Keys, a.OnMessageReceived)
	a.Messenger.SetAttachmentSaver(a.SaveAttachment)
	a.Messenger.SetContactHandler(a.OnContactRequest)
	a.Messenger.SetFileOfferHandler(a.onFileOffer)
	a.Messenger.SetFileResponseHandler(a.onFileResponse)
	a.Messenger.SetProfileUpdateHandler(a.onProfileUpdate)
	a.Messenger.SetProfileRequestHandler(a.onProfileRequest)

	if err := a.Messenger.Start(a.Ctx); err != nil {
		a.SetNetworkStatus(StatusError)
		return
	}

	a.SetNetworkStatus(StatusOnline)
}

// formatAvatarURL преобразует локальный путь в URL для фронтенда для текущего пользователя
func (a *AppCore) formatAvatarURL(path string) string {
	if a.Identity == nil {
		return a.formatProfileAvatarURL("", path)
	}
	return a.formatProfileAvatarURL(a.Identity.Keys.UserID, path)
}

// formatProfileAvatarURL преобразует путь в URL с учетом UserID
func (a *AppCore) formatProfileAvatarURL(userID, path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "data:") {
		return path
	}
	filename := filepath.Base(path)
	// Если это абсолютный путь, берем только имя файла и добавляем префикс /avatars/
	// если он лежит в папке avatars или /secure/ если в media
	if strings.Contains(path, "avatars") {
		if userID != "" {
			return fmt.Sprintf("/avatars/%s/%s", userID, filename)
		}
		return "/avatars/unknown/" + filename
	}
	return "/secure/" + filename
}

// onProfileUpdate обрабатывает входящее обновление профиля от контакта
func (a *AppCore) onProfileUpdate(senderPubKey, nickname, bio string, avatar []byte) {
	if a.Repo == nil {
		return
	}

	if len(avatar) > MaxAvatarSize {
		log.Printf("[AppCore] Ignored large avatar (%d bytes) from %s", len(avatar), senderPubKey[:8])
		avatar = nil // Не сохраняем слишком большую аватарку
	}

	contact, _ := a.Repo.GetContactByPublicKey(a.Ctx, senderPubKey)
	if contact == nil {
		return
	}

	contact.Nickname = nickname
	contact.Bio = bio

	if len(avatar) > 0 {
		// Сохраняем аватар (НЕ зашифровано!)
		filename := fmt.Sprintf("avatar_%s.png", senderPubKey[:8])
		path, err := a.SaveAvatar(filename, avatar)
		if err == nil {
			contact.Avatar = path
		}
	}

	a.Repo.SaveContact(a.Ctx, contact)
	a.Emitter.Emit("contact_updated")
}

// onProfileRequest обрабатывает запрос нашего профиля
func (a *AppCore) onProfileRequest(requestorPubKey string) {
	if a.Repo == nil || a.Messenger == nil {
		return
	}

	user, _ := a.Repo.GetMyProfile(a.Ctx)
	if user == nil {
		return
	}

	var avatarData []byte
	if user.Avatar != "" {
		data, err := os.ReadFile(user.Avatar)
		if err == nil {
			if len(data) <= MaxAvatarSize {
				avatarData = data
			} else {
				log.Printf("[AppCore] Our avatar is too large to send (%d bytes)", len(data))
			}
		}
	}

	// Отправляем наш профиль в ответ
	// Нам нужен адрес контакта, чтобы отправить сообщение.
	// Но у нас есть только PubKey. Ищем контакт в БД.
	contact, _ := a.Repo.GetContactByPublicKey(a.Ctx, requestorPubKey)
	if contact != nil {
		a.Messenger.SendProfileUpdate(contact.I2PAddress, user.Nickname, user.Bio, avatarData)
	}
}

// ─── Обработчики входящих сообщений ─────────────────────────────────────────

// OnMessageReceived — обработчик входящих сообщений.
// Полная логика: сохранение в БД, автосоздание контактов, события фронтенду.
func (a *AppCore) OnMessageReceived(msg *core.Message, senderPubKey, senderAddr string) {
	if a.Repo == nil {
		return
	}

	contact, _ := a.Repo.GetContactByPublicKey(a.Ctx, senderPubKey)
	if contact == nil {
		// Создаем контакт если неизвестен
		newChatID := identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, senderPubKey)
		contact = &core.Contact{
			ID:         uuid.New().String(),
			PublicKey:  senderPubKey,
			Nickname:   "Unknown " + senderPubKey[:8],
			I2PAddress: senderAddr,
			ChatID:     newChatID,
			AddedAt:    time.Now(),
		}
		a.Repo.SaveContact(a.Ctx, contact)
		a.Emitter.Emit("contact_updated")
		// Запрашиваем профиль у нового контакта
		if a.Messenger != nil {
			go a.Messenger.SendProfileRequest(senderAddr)
		}
	}

	msg.ChatID = contact.ChatID

	if err := a.Repo.SaveMessage(a.Ctx, msg); err != nil {
		return
	}

	var replyToIDStr string
	if msg.ReplyToID != nil {
		replyToIDStr = *msg.ReplyToID
	}
	replyPreview := a.getReplyPreview(replyToIDStr, contact)

	// Формируем вложения для фронтенда
	attachments := make([]map[string]interface{}, 0, len(msg.Attachments))
	for _, att := range msg.Attachments {
		attachments = append(attachments, map[string]interface{}{
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
		"ReplyToID":    msg.ReplyToID,
		"ReplyPreview": replyPreview,
		"Attachments":  attachments,
	})

	if !msg.IsOutgoing {
		// Подавляем уведомление, если приложение видимо, в фокусе и открыт именно этот чат
		if !a.IsVisible || !(a.IsFocused && a.ActiveChatID == msg.ChatID) {
			go a.SendNotification(contact.Nickname, msg.Content, msg.ContentType)
		}
		go a.UpdateUnreadCount()
	}
}

// OnContactRequest — обработчик запросов дружбы.
func (a *AppCore) OnContactRequest(pubKey, nickname, i2pAddress string) {
	a.Emitter.Emit("new_contact", map[string]interface{}{
		"nickname": nickname,
	})
}

// UpdateUnreadCount обновляет счётчик непрочитанных.
func (a *AppCore) UpdateUnreadCount() {
	if a.Repo == nil {
		return
	}
	count, err := a.Repo.GetUnreadCount(a.Ctx)
	if err != nil {
		return
	}
	a.Emitter.Emit("unread_count", count)
}

// SendNotification формирует и отправляет системное уведомление.
func (a *AppCore) SendNotification(senderName, content, contentType string) {
	title := fmt.Sprintf("TeleGhost - %s", senderName)
	message := content

	switch contentType {
	case "file_offer":
		message = "📎 Отправил(а) файл"
	case "mixed":
		if content == "" {
			message = "📷 Отправил(а) изображение"
		} else {
			message = fmt.Sprintf("📷 %s", content)
		}
	case "text":
		if len(message) > 100 {
			message = message[:97] + "..."
		}
	}

	a.Platform.Notify(title, message)
}

// getReplyPreview ищет исходное сообщение и формирует превью для ответа
func (a *AppCore) getReplyPreview(replyToID string, contact *core.Contact) *ReplyPreview {
	if replyToID == "" || a.Repo == nil {
		return nil
	}

	orig, _ := a.Repo.GetMessage(a.Ctx, replyToID)
	if orig == nil {
		return nil
	}

	author := "Неизвестный"
	if orig.IsOutgoing {
		author = "Я"
	} else if contact != nil {
		author = contact.Nickname
	}

	return &ReplyPreview{
		AuthorName: author,
		Content:    orig.Content,
	}
}
