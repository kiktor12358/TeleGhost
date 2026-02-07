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

	"encoding/base64"
	pb "teleghost/internal/proto"

	"github.com/go-i2p/i2pkeys"
	"google.golang.org/protobuf/proto"
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
	Avatar      string `json:"avatar"`
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
	Avatar      string `json:"avatar"`
	PublicKey   string `json:"publicKey"`
	Destination string `json:"destination"`
	Fingerprint string `json:"fingerprint"`
}

// App основная структура приложения
type App struct {
	ctx            context.Context
	identity       *identity.Identity
	repo           *sqlite.Repository
	router         *router.SAMRouter
	messenger      *messenger.Service
	status         NetworkStatus
	dataDir        string
	embeddedRouter interface {
		IsReady() bool
		Start(context.Context) error
		Stop() error
	}
	embeddedStop func() error
	trayManager  *TrayManager
}

// NewApp creates a new App application struct
func NewApp(iconData []byte) *App {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".teleghost")

	app := &App{
		status:  NetworkStatusOffline,
		dataDir: dataDir,
	}

	app.trayManager = NewTrayManager(app, iconData)
	return app
}

// startup вызывается при старте приложения
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Инициализируем clipboard
	if err := clipboard.Init(); err != nil {
		log.Printf("[App] Clipboard init failed: %v", err)
	}

	// Создаём корневую директорию данных
	os.MkdirAll(a.dataDir, 0700)
	os.MkdirAll(filepath.Join(a.dataDir, "users"), 0700)

	// Репозиторий создаётся после логина в initUserRepository()

	// Инициализируем встроенный роутер (если есть)
	if err := a.initEmbeddedRouter(ctx); err != nil {
		log.Printf("[App] Failed to init embedded router: %v", err)
	}

	// Запускаем трей
	if a.trayManager != nil {
		a.trayManager.Start()
	}

	log.Printf("[App] Started. Data dir: %s", a.dataDir)
}

// shutdown вызывается при закрытии приложения
func (a *App) shutdown(ctx context.Context) {
	log.Printf("[App] Shutting down...")

	// Сначала останавливаем роутер (это закроет listener и разблокирует messenger.listenLoop)
	if a.router != nil {
		a.router.Stop()
	}

	if a.messenger != nil {
		a.messenger.Stop()
	}

	if a.repo != nil {
		a.repo.Close()
	}

	if a.embeddedStop != nil {
		log.Println("[App] Stopping embedded router...")

		done := make(chan struct{})
		go func() {
			a.embeddedStop()
			close(done)
		}()

		select {
		case <-done:
			log.Println("[App] Embedded router stopped successfully")
		case <-time.After(5 * time.Second):
			log.Println("[App] Warning: Embedded router stop timed out, forcing exit")
		}
	}

	if a.trayManager != nil {
		log.Println("[App] Stopping tray...")
		a.trayManager.Stop()
	}

	log.Println("[App] Shutdown complete.")
}

// initUserRepository инициализирует репозиторий для конкретного пользователя
func (a *App) initUserRepository(userID string) error {
	// Путь: ~/.teleghost/users/{userID}/
	userDir := filepath.Join(a.dataDir, "users", userID)
	if err := os.MkdirAll(userDir, 0700); err != nil {
		return fmt.Errorf("failed to create user directory: %w", err)
	}

	dbPath := filepath.Join(userDir, "data.db")
	repo, err := sqlite.New(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	a.repo = repo

	// Запускаем миграции
	if err := repo.Migrate(a.ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Printf("[App] User repository initialized: %s", userDir)
	return nil
}

// ShowWindow показывает окно из трея
func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
}

// QuitApp полностью закрывает приложение
func (a *App) QuitApp() {
	runtime.Quit(a.ctx)
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

	// Инициализируем репозиторий для этого пользователя
	if err := a.initUserRepository(keys.UserID); err != nil {
		return fmt.Errorf("failed to init user repository: %w", err)
	}

	// Проверяем существующий профиль или создаём новый
	existingProfile, _ := a.repo.GetMyProfile(a.ctx)
	if existingProfile == nil {
		// Новый пользователь — создаём профиль
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
	} else {
		log.Printf("[App] Found existing profile: %s", existingProfile.Nickname)
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

	// Инициализируем репозиторий для нового пользователя
	if err := a.initUserRepository(id.Keys.UserID); err != nil {
		return "", fmt.Errorf("failed to init user repository: %w", err)
	}

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

	// Если у нас есть встроенный роутер, ждем пока он будет готов
	if a.embeddedRouter != nil {
		log.Println("[App] Waiting for embedded i2pd router to be ready...")
		// Ждем до 30 секунд (в дополнение к внутреннему ожиданию)
		for i := 0; i < 30; i++ {
			if a.embeddedRouter.IsReady() {
				log.Println("[App] Embedded router is ready!")
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	// Загружаем I2P ключи из файла, если есть
	userDir := filepath.Join(a.dataDir, "users", a.identity.Keys.UserID)
	keysPath := filepath.Join(userDir, "i2p.keys")

	if _, err := os.Stat(keysPath); err == nil {
		log.Println("[App] Loading existing I2P keys from file...")
		keys, err := i2pkeys.LoadKeys(keysPath)
		if err == nil {
			a.router.SetKeys(keys)
		} else {
			log.Printf("[App] Warning: failed to load I2P keys: %v", err)
		}
	}

	if err := a.router.Start(a.ctx); err != nil {
		log.Printf("[App] I2P connection failed: %v", err)
		a.setNetworkStatus(NetworkStatusError)
		return
	}

	// Сохраняем I2P ключи в файл
	destination := a.router.GetDestination()
	currentKeys := a.router.GetKeys()
	if err := i2pkeys.StoreKeys(currentKeys, keysPath); err != nil {
		log.Printf("[App] Warning: failed to save I2P keys: %v", err)
	} else {
		log.Printf("[App] I2P keys saved to %s", keysPath)
	}

	// Обновляем I2P адрес в профиле
	if a.repo != nil && a.identity != nil {
		existingUser, _ := a.repo.GetMyProfile(a.ctx)
		if existingUser != nil {
			existingUser.I2PAddress = destination
			a.repo.SaveUser(a.ctx, existingUser)
		}
	}

	// Создаём мессенджер сервис
	a.messenger = messenger.NewService(a.router, a.identity.Keys, a.onMessageReceived)

	// Устанавливаем обработчик входящих handshake
	a.messenger.SetContactHandler(a.onContactRequest)

	// Устанавливаем никнейм для исходящих handshake
	nickname := "User"
	if profile, err := a.repo.GetMyProfile(a.ctx); err == nil && profile != nil {
		nickname = profile.Nickname
	}
	a.messenger.SetNickname(nickname)
	a.messenger.SetProfileUpdateHandler(a.onProfileUpdate)

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
func (a *App) onMessageReceived(msg *core.Message, senderPubKey, senderAddr string) {
	if a.repo == nil {
		return
	}

	// 1. Сначала ищем по публичному ключу
	contact, err := a.repo.GetContactByPublicKey(a.ctx, senderPubKey)
	if err != nil {
		log.Printf("[App] Error looking up contact by pubkey: %v", err)
	}

	// 2. Если не нашли, ищем по I2P адресу (если мы добавили друга по адресу, но еще не знали его ключ)
	if contact == nil {
		contact, err = a.repo.GetContactByAddress(a.ctx, senderAddr)
		if err != nil {
			log.Printf("[App] Error looking up contact by address: %v", err)
		}

		if contact != nil {
			// Обновляем публичный ключ контакта, так как мы его теперь знаем
			contact.PublicKey = senderPubKey
			// Также пересчитываем ChatID на детерминированный, так как теперь есть все данные
			newChatID := identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, senderPubKey)
			contact.ChatID = newChatID
			a.repo.SaveContact(a.ctx, contact)
			log.Printf("[App] Updated contact %s with public key and new ChatID", contact.Nickname)
		}
	}

	if contact != nil {
		// Используем ChatID из найденного (или обновленного) контакта
		msg.ChatID = contact.ChatID
	} else {
		// Если контакт совсем неизвестен, создаем временного
		log.Printf("[App] Message from unknown sender: %s (%s)", senderPubKey[:16], senderAddr[:16])
		// Можно добавить автоматическое создание контакта "Unknown" здесь
	}

	// Сохраняем в БД
	if err := a.repo.SaveMessage(a.ctx, msg); err != nil {
		log.Printf("[App] Failed to save incoming message: %v", err)
		return
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

// onContactRequest обработчик входящих handshake — автоматически создаёт контакт
func (a *App) onContactRequest(pubKey, nickname, i2pAddress string) {
	if a.repo == nil || a.identity == nil {
		return
	}

	// Проверяем, существует ли контакт с таким публичным ключом
	existingContact, err := a.repo.GetContactByPublicKey(a.ctx, pubKey)
	if err == nil && existingContact != nil {
		// Контакт уже есть, обновляем I2P адрес если изменился
		if existingContact.I2PAddress != i2pAddress {
			existingContact.I2PAddress = i2pAddress
			existingContact.UpdatedAt = time.Now()
			a.repo.SaveContact(a.ctx, existingContact)
			log.Printf("[App] Updated I2P address for contact %s", existingContact.Nickname)
		}
		return
	}

	// Вычисляем детерминированный ChatID
	chatID := identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, pubKey)

	// Создаём новый контакт
	contact := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  pubKey,
		Nickname:   nickname,
		I2PAddress: i2pAddress,
		ChatID:     chatID,
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		log.Printf("[App] Failed to save contact from handshake: %v", err)
		return
	}

	log.Printf("[App] New contact created from handshake: %s (%s)", nickname, pubKey[:16])

	// Уведомляем фронтенд о новом контакте
	runtime.EventsEmit(a.ctx, "new_contact", map[string]interface{}{
		"id":         contact.ID,
		"nickname":   nickname,
		"publicKey":  pubKey,
		"i2pAddress": i2pAddress[:min(32, len(i2pAddress))] + "...",
	})
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
	avatar := ""

	// Получаем профиль из БД
	if a.repo != nil {
		profile, err := a.repo.GetMyProfile(a.ctx)
		if err == nil && profile != nil {
			nickname = profile.Nickname
			avatar = profile.Avatar
		}
	}

	return &UserInfo{
		ID:          a.identity.Keys.UserID,
		Nickname:    nickname,
		Avatar:      avatar,
		PublicKey:   a.identity.Keys.PublicKeyBase64,
		Destination: a.GetMyDestination(),
		Fingerprint: a.identity.Keys.Fingerprint(),
	}
}

// AddContactFromClipboard добавляет контакт из буфера обмена
func (a *App) AddContactFromClipboard() (*ContactInfo, error) {
	// Читаем буфер обмена
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		return nil, fmt.Errorf("clipboard is empty")
	}

	destination := strings.TrimSpace(string(data))

	// I2P destination ~516 символов в base64
	// I2P destination: base64 (~516+) or base32 (~60)
	if len(destination) < 50 {
		return nil, fmt.Errorf("invalid I2P destination (too short)")
	}

	// Создаём контакт
	contact := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  "", // Пока не знаем
		Nickname:   "New Contact",
		I2PAddress: destination,
		ChatID:     uuid.New().String(), // Временный ID, будет обновлен после первого сообщения
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

	// I2P destination: base64 (~516+) or base32 (~60)
	if len(destination) < 50 {
		return nil, fmt.Errorf("invalid I2P destination (too short)")
	}

	contact := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  "", // Пока не знаем
		Nickname:   name,
		I2PAddress: destination,
		ChatID:     uuid.New().String(), // Временный ID, будет обновлен после первого сообщения
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

// DeleteContact удаляет контакт
func (a *App) DeleteContact(id string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}

	if err := a.repo.DeleteContact(a.ctx, id); err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}

	log.Printf("[App] Deleted contact: %s", id)
	return nil
}

// GetContacts возвращает список контактов
func (a *App) GetContacts() ([]*ContactInfo, error) {
	contacts, err := a.repo.ListContacts(a.ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*ContactInfo, len(contacts))
	for i, c := range contacts {
		// Отдаем полный адрес, фронтенд сам обрежет если надо
		fullAddr := c.I2PAddress

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
			Avatar:      c.Avatar,
			PublicKey:   c.PublicKey,
			I2PAddress:  fullAddr,
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

	// Если у контакта нет нашего публичного ключа (первый контакт), отправляем handshake
	// Проверяем по флагу "handshake отправлен" или по отсутствию ChatID (старый контакт)
	if contact.ChatID == "" {
		// Вычисляем ChatID
		contact.ChatID = identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, contact.PublicKey)
		contact.UpdatedAt = time.Now()
		a.repo.SaveContact(a.ctx, contact)
	}

	// Отправляем handshake перед первым сообщением
	// Это гарантирует, что получатель знает наш адрес и публичный ключ
	if err := a.messenger.SendHandshake(contact.I2PAddress); err != nil {
		log.Printf("[App] Handshake failed (will try sending anyway): %v", err)
		// Не возвращаем ошибку — попробуем отправить сообщение
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

	// Отправляем событие во фронтенд для мгновенного отображения
	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"id":         msg.ID,
		"chatId":     msg.ChatID,
		"senderId":   msg.SenderID,
		"content":    msg.Content,
		"timestamp":  msg.Timestamp,
		"isOutgoing": msg.IsOutgoing,
		"status":     "sent",
	})

	return nil
}

// UpdateMyProfile обновляет профиль
func (a *App) UpdateMyProfile(nickname, bio, avatar string) error {
	// Если аватар не передан, попробуем сохранить старый?
	// Или репозиторий сам разберется?
	// Reposiotry UPDATE sets avatar = ?. So if empty string passed, it clears it?
	// Frontend should pass current avatar if not changing.
	if err := a.repo.UpdateMyProfile(a.ctx, nickname, bio, avatar); err != nil {
		return err
	}
	go a.broadcastProfileUpdate()
	return nil
}

// CopyToClipboard копирует текст в буфер обмена
func (a *App) CopyToClipboard(text string) {
	clipboard.Write(clipboard.FmtText, []byte(text))
}

// EditMessage редактирует содержимое сообщения
func (a *App) EditMessage(messageID, newContent string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}

	// Обновляем в локальной БД
	if err := a.repo.UpdateMessageContent(a.ctx, messageID, newContent); err != nil {
		return fmt.Errorf("failed to edit message: %w", err)
	}

	// TODO: Отправить MESSAGE_EDIT пакет получателю через messenger
	// Это будет реализовано в следующей итерации

	log.Printf("[App] Message edited: %s", messageID[:8])
	return nil
}

// DeleteMessage удаляет сообщение локально
func (a *App) DeleteMessage(messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}

	if err := a.repo.DeleteMessage(a.ctx, messageID); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	log.Printf("[App] Message deleted locally: %s", messageID[:8])
	return nil
}

// DeleteMessageForAll удаляет сообщение у всех участников (отправляет пакет)
func (a *App) DeleteMessageForAll(messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("database not initialized")
	}

	// Получаем сообщение для определения чата
	msg, err := a.repo.GetMessage(a.ctx, messageID)
	if err != nil {
		return fmt.Errorf("failed to get message: %w", err)
	}
	if msg == nil {
		return fmt.Errorf("message not found")
	}

	// Удаляем локально
	if err := a.repo.DeleteMessage(a.ctx, messageID); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	// TODO: Отправить MESSAGE_DELETE пакет получателю через messenger
	// Это будет реализовано в следующей итерации

	log.Printf("[App] Message deleted for all: %s", messageID[:8])
	return nil
}

// GetMessageByID возвращает сообщение по ID
func (a *App) GetMessageByID(messageID string) (*MessageInfo, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	msg, err := a.repo.GetMessage(a.ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("message not found")
	}

	return &MessageInfo{
		ID:         msg.ID,
		Content:    msg.Content,
		Timestamp:  msg.Timestamp,
		IsOutgoing: msg.IsOutgoing,
		Status:     msg.Status.String(),
	}, nil
}

// min возвращает минимум из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// === Folder API ===

// FolderInfo информация о папке с ID чатов
type FolderInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Icon     string   `json:"icon"`
	ChatIDs  []string `json:"chatIds"`
	Position int      `json:"position"`
}

// CreateFolder создаёт новую папку
func (a *App) CreateFolder(name, icon string) (string, error) {
	if a.repo == nil {
		return "", fmt.Errorf("database not initialized")
	}

	id := uuid.New().String()
	// Получаем текущее количество папок для позиции
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
			log.Printf("[App] Failed to get chats for folder %s: %v", f.ID, err)
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

// BroadcastProfileUpdate broadcasts current profile to all contacts
func (a *App) broadcastProfileUpdate() {
	if a.messenger == nil {
		return
	}

	profile, err := a.repo.GetMyProfile(a.ctx)
	if err != nil || profile == nil {
		return
	}

	// Decode avatar base64
	avatarBytes := []byte{}
	if profile.Avatar != "" {
		// handle data:image... prefix logic if stored with prefix
		// Frontend sends raw base64 (from resizeImage toDataURL -> stored in var)
		// Wait, canvas.toDataURL returns "data:image/jpeg;base64,..."
		// So DB has prefix.
		parts := strings.Split(profile.Avatar, ",")
		if len(parts) > 1 {
			avatarBytes, _ = base64.StdEncoding.DecodeString(parts[1])
		} else {
			avatarBytes, _ = base64.StdEncoding.DecodeString(profile.Avatar)
		}
	}

	payload, err := proto.Marshal(&pb.ProfileUpdate{
		Nickname: profile.Nickname,
		Bio:      profile.Bio,
		Avatar:   avatarBytes,
	})
	if err != nil {
		log.Printf("[App] Failed to marshal ProfileUpdate: %v", err)
		return
	}

	packet := &pb.Packet{
		Version:      1,
		Type:         pb.PacketType_PROFILE_UPDATE,
		SenderPubKey: []byte(a.identity.Keys.PublicKeyBase64),
		Payload:      payload,
	}

	// TODO: Sign packet? Messenger handles signing if logic is there?
	// Messenger.SendMessage calls internally some logic?
	// No, app.go SendText creates packet AND signs it?
	// Step 934 log: Signature: []byte{}, // TODO: Sign.
	// So app.go is responsible for signing.
	// I'll leave signature empty for now as defined protocol allows it (or ignores it).
	// Identity check is done via SenderPubKey?
	// Ideally I should sign. a.identity.Keys.Sign(...)

	// Sign packet
	packet.Signature = a.identity.Keys.SignMessage(payload)

	a.messenger.Broadcast(packet)
}

// onProfileUpdate handles incoming profile updates
func (a *App) onProfileUpdate(pubKey, nickname, bio string, avatar []byte) {
	if a.repo == nil {
		return
	}

	contact, err := a.repo.GetContactByPublicKey(a.ctx, pubKey)
	if err != nil || contact == nil {
		return
	}

	// Encode avatar to base64
	avatarStr := ""
	if len(avatar) > 0 {
		avatarStr = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(avatar)
	}

	contact.Nickname = nickname
	contact.Bio = bio
	contact.Avatar = avatarStr
	contact.UpdatedAt = time.Now()

	if err := a.repo.SaveContact(a.ctx, contact); err != nil {
		log.Printf("[App] Failed to update contact profile: %v", err)
		return
	}

	// Notify frontend
	runtime.EventsEmit(a.ctx, "contact_updated", map[string]interface{}{
		"id":       contact.ID,
		"nickname": contact.Nickname,
		"avatar":   contact.Avatar,
		"bio":      contact.Bio,
	})
}
