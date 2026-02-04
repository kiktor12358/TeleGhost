package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/clipboard"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
	"teleghost/internal/network/messenger"
	"teleghost/internal/network/router"
	"teleghost/internal/repository/sqlite"
)

// NetworkStatus статус подключения к I2P
type NetworkStatus string

const (
	NetworkStatusOffline    NetworkStatus = "offline"
	NetworkStatusConnecting NetworkStatus = "connecting"
	NetworkStatusOnline     NetworkStatus = "online"
	NetworkStatusError      NetworkStatus = "error"
)

// ContactInfo информация о контакте для фронтенда
type ContactInfo struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	PublicKey   string `json:"publicKey"`
	I2PAddress  string `json:"i2pAddress"`
	LastMessage string `json:"lastMessage"`
	LastSeen    string `json:"lastSeen"`
	IsOnline    bool   `json:"isOnline"`
}

// MessageInfo сообщение для фронтенда
type MessageInfo struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Timestamp  int64  `json:"timestamp"`
	IsOutgoing bool   `json:"isOutgoing"`
	Status     string `json:"status"`
}

// UserInfo информация о текущем пользователе
type UserInfo struct {
	ID          string `json:"id"`
	Nickname    string `json:"nickname"`
	PublicKey   string `json:"publicKey"`
	Destination string `json:"destination"`
	Fingerprint string `json:"fingerprint"`
}

// App основная структура приложения
type App struct {
	ctx          context.Context
	identity     *identity.Identity
	repo         *sqlite.Repository
	router       *router.SAMRouter
	messenger    *messenger.Service
	status       NetworkStatus
	dataDir      string
	embeddedStop func() error
}

// NewApp creates a new App application struct
func NewApp() *App {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".teleghost")

	return &App{
		status:  NetworkStatusOffline,
		dataDir: dataDir,
	}
}

// startup вызывается при старте приложения
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Инициализируем clipboard
	if err := clipboard.Init(); err != nil {
		log.Printf("[App] Clipboard init failed: %v", err)
	}

	// Создаём директорию данных
	os.MkdirAll(a.dataDir, 0700)

	// Инициализируем репозиторий
	dbPath := filepath.Join(a.dataDir, "teleghost.db")
	repo, err := sqlite.New(dbPath)
	if err != nil {
		log.Printf("[App] Database error: %v", err)
		return
	}
	a.repo = repo

	// Запускаем миграции
	if err := repo.Migrate(ctx); err != nil {
		log.Printf("[App] Migration error: %v", err)
	}

	// Проверяем есть ли сохранённый профиль
	profile, err := repo.GetMyProfile(ctx)
	if err == nil && profile != nil {
		log.Printf("[App] Found existing profile: %s", profile.Nickname)
	}

	// Инициализируем встроенный роутер (если есть)
	if err := a.initEmbeddedRouter(ctx); err != nil {
		log.Printf("[App] Failed to init embedded router: %v", err)
	}

	log.Printf("[App] Started. Data dir: %s", a.dataDir)
}

// shutdown вызывается при закрытии приложения
func (a *App) shutdown(ctx context.Context) {
	log.Printf("[App] Shutting down...")

	if a.messenger != nil {
		a.messenger.Stop()
	}

	if a.router != nil {
		a.router.Stop()
	}

	if a.repo != nil {
		a.repo.Close()
	}

	if a.embeddedStop != nil {
		a.embeddedStop()
	}
}

// === Методы для фронтенда ===

// Login авторизация по seed-фразе
func (a *App) Login(seedPhrase string) error {
	seedPhrase = strings.TrimSpace(seedPhrase)

	// Валидируем мнемонику
	if !identity.ValidateMnemonic(seedPhrase) {
		return fmt.Errorf("invalid seed phrase")
	}

	// Восстанавливаем ключи
	keys, err := identity.RecoverKeys(seedPhrase)
	if err != nil {
		return fmt.Errorf("failed to recover keys: %w", err)
	}

	a.identity = &identity.Identity{
		Mnemonic: seedPhrase,
		Keys:     keys,
	}

	// Сохраняем/обновляем профиль в БД
	user := &core.User{
		ID:         keys.UserID,
		PublicKey:  keys.PublicKeyBase64,
		PrivateKey: keys.SigningPrivateKey,
		Mnemonic:   seedPhrase,
		Nickname:   "User",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.repo.SaveUser(a.ctx, user); err != nil {
		return fmt.Errorf("failed to save profile: %w", err)
	}

	// Запускаем I2P подключение
	go a.connectToI2P()

	log.Printf("[App] Logged in as %s", keys.UserID)
	return nil
}

// CreateAccount создаёт новый аккаунт
func (a *App) CreateAccount() (string, error) {
	id, err := identity.GenerateNewIdentity()
	if err != nil {
		return "", fmt.Errorf("failed to generate identity: %w", err)
	}

	a.identity = id

	// Сохраняем профиль
	user := &core.User{
		ID:         id.Keys.UserID,
		PublicKey:  id.Keys.PublicKeyBase64,
		PrivateKey: id.Keys.SigningPrivateKey,
		Mnemonic:   id.Mnemonic,
		Nickname:   "User",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.repo.SaveUser(a.ctx, user); err != nil {
		return "", fmt.Errorf("failed to save profile: %w", err)
	}

	// Запускаем I2P подключение
	go a.connectToI2P()

	log.Printf("[App] Created new account: %s", id.Keys.UserID)
	return id.Mnemonic, nil
}

// connectToI2P подключение к I2P сети
func (a *App) connectToI2P() {
	a.setNetworkStatus(NetworkStatusConnecting)

	// Создаём роутер
	cfg := router.DefaultConfig()
	a.router = router.NewSAMRouter(cfg)

	// Запускаем роутер
	if err := a.router.Start(a.ctx); err != nil {
		log.Printf("[App] I2P connection failed: %v", err)
		a.setNetworkStatus(NetworkStatusError)
		return
	}

	// Обновляем I2P адрес в профиле
	destination := a.router.GetDestination()
	if a.repo != nil && a.identity != nil {
		user := &core.User{
			ID:         a.identity.Keys.UserID,
			PublicKey:  a.identity.Keys.PublicKeyBase64,
			I2PAddress: destination,
		}
		a.repo.SaveUser(a.ctx, user)
	}

	// Создаём мессенджер сервис
	a.messenger = messenger.NewService(a.router, a.identity.Keys, a.onMessageReceived)

	// Запускаем мессенджер
	if err := a.messenger.Start(a.ctx); err != nil {
		log.Printf("[App] Messenger start failed: %v", err)
		a.setNetworkStatus(NetworkStatusError)
		return
	}

	a.setNetworkStatus(NetworkStatusOnline)
	log.Printf("[App] Connected to I2P. Destination: %s...", destination[:32])
}

// onMessageReceived обработчик входящих сообщений
func (a *App) onMessageReceived(msg *core.Message, senderPubKey string) {
	// Сохраняем в БД
	if a.repo != nil {
		a.repo.SaveMessage(a.ctx, msg)
	}

	// Отправляем событие во фронтенд
	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"id":         msg.ID,
		"chatId":     msg.ChatID,
		"senderId":   msg.SenderID,
		"content":    msg.Content,
		"timestamp":  msg.Timestamp,
		"isOutgoing": msg.IsOutgoing,
	})

	log.Printf("[App] New message received: %s", msg.Content[:min(20, len(msg.Content))])
}

// setNetworkStatus обновляет статус сети и уведомляет фронтенд
func (a *App) setNetworkStatus(status NetworkStatus) {
	a.status = status
	runtime.EventsEmit(a.ctx, "network_status", string(status))
}

// GetNetworkStatus возвращает текущий статус сети
func (a *App) GetNetworkStatus() string {
	return string(a.status)
}

// GetMyDestination возвращает I2P адрес для копирования
func (a *App) GetMyDestination() string {
	if a.router == nil {
		return ""
	}
	return a.router.GetDestination()
}

// GetMyInfo возвращает информацию о текущем пользователе
func (a *App) GetMyInfo() *UserInfo {
	if a.identity == nil {
		return nil
	}

	nickname := "User"

	// Получаем профиль из БД
	if a.repo != nil {
		profile, err := a.repo.GetMyProfile(a.ctx)
		if err == nil && profile != nil {
			nickname = profile.Nickname
		}
	}

	return &UserInfo{
		ID:          a.identity.Keys.UserID,
		Nickname:    nickname,
		PublicKey:   a.identity.Keys.PublicKeyBase64,
		Destination: a.GetMyDestination(),
		Fingerprint: a.identity.Keys.Fingerprint(),
	}
}

// AddContactFromClipboard добавляет контакт из буфера обмена
func (a *App) AddContactFromClipboard() (*ContactInfo, error) {
	// Читаем буфер обмена
	data := clipboard.Read(clipboard.FmtText)
	if data == nil || len(data) == 0 {
		return nil, fmt.Errorf("clipboard is empty")
	}

	destination := strings.TrimSpace(string(data))

	// I2P destination ~516 символов в base64
	if len(destination) < 300 {
		return nil, fmt.Errorf("invalid I2P destination")
	}

	// Создаём контакт
	contact := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  "", // Будет получен при handshake
		Nickname:   "New Contact",
		I2PAddress: destination,
		ChatID:     uuid.New().String(),
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		return nil, fmt.Errorf("failed to save contact: %w", err)
	}

	log.Printf("[App] Added contact: %s...", destination[:32])

	return &ContactInfo{
		ID:         contact.ID,
		Nickname:   contact.Nickname,
		I2PAddress: destination[:32] + "...",
	}, nil
}

// AddContact добавляет контакт по I2P адресу
func (a *App) AddContact(name, destination string) (*ContactInfo, error) {
	destination = strings.TrimSpace(destination)

	if len(destination) < 300 {
		return nil, fmt.Errorf("invalid I2P destination")
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
	}, nil
}

// GetContacts возвращает список контактов
func (a *App) GetContacts() ([]*ContactInfo, error) {
	contacts, err := a.repo.ListContacts(a.ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*ContactInfo, len(contacts))
	for i, c := range contacts {
		shortAddr := c.I2PAddress
		if len(shortAddr) > 32 {
			shortAddr = shortAddr[:32] + "..."
		}

		// Получаем последнее сообщение
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
			PublicKey:   c.PublicKey,
			I2PAddress:  shortAddr,
			LastMessage: lastMsg,
			LastSeen:    c.LastSeen.Format("15:04"),
		}
	}

	return result, nil
}

// GetMessages возвращает сообщения чата с контактом
func (a *App) GetMessages(contactID string, limit, offset int) ([]*MessageInfo, error) {
	// Получаем контакт для chatID
	contact, err := a.repo.GetContact(a.ctx, contactID)
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, fmt.Errorf("contact not found")
	}

	messages, err := a.repo.GetChatHistory(a.ctx, contact.ChatID, limit, offset)
	if err != nil {
		return nil, err
	}

	result := make([]*MessageInfo, len(messages))
	for i, m := range messages {
		status := "sent"
		switch m.Status {
		case core.MessageStatusDelivered:
			status = "delivered"
		case core.MessageStatusRead:
			status = "read"
		case core.MessageStatusFailed:
			status = "failed"
		}

		result[i] = &MessageInfo{
			ID:         m.ID,
			Content:    m.Content,
			Timestamp:  m.Timestamp,
			IsOutgoing: m.IsOutgoing,
			Status:     status,
		}
	}

	return result, nil
}

// SendText отправляет текстовое сообщение
func (a *App) SendText(contactID, text string) error {
	if a.messenger == nil {
		return fmt.Errorf("not connected to I2P")
	}

	// Получаем контакт
	contact, err := a.repo.GetContact(a.ctx, contactID)
	if err != nil {
		return err
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	// Отправляем сообщение
	if err := a.messenger.SendTextMessage(contact.I2PAddress, contact.ChatID, text); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}

	// Сохраняем в БД
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

	if err := a.repo.SaveMessage(a.ctx, msg); err != nil {
		log.Printf("[App] Failed to save outgoing message: %v", err)
	}

	return nil
}

// UpdateMyProfile обновляет профиль
func (a *App) UpdateMyProfile(nickname, bio string) error {
	return a.repo.UpdateMyProfile(a.ctx, nickname, bio, "")
}

// CopyToClipboard копирует текст в буфер обмена
func (a *App) CopyToClipboard(text string) {
	clipboard.Write(clipboard.FmtText, []byte(text))
}

// min возвращает минимум из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
