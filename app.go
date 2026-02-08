package main

import (
	"context"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
	"teleghost/internal/network/messenger"
	"teleghost/internal/network/router"
	"teleghost/internal/repository/sqlite"
	"teleghost/internal/utils"

	"encoding/base64"
	pb "teleghost/internal/proto"

	"github.com/go-i2p/i2pkeys"
	"golang.design/x/clipboard"
	"google.golang.org/protobuf/proto"

	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"github.com/nfnt/resize"
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

	transferMu       sync.RWMutex
	pendingTransfers map[string]*PendingTransfer // messageID -> transfer info
}

type PendingTransfer struct {
	Destination string
	ChatID      string
	Files       []string
	MessageID   string
	Timestamp   int64
}

// NewApp creates a new App application struct
func NewApp(iconData []byte) *App {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".teleghost")

	app := &App{
		status:           NetworkStatusOffline,
		dataDir:          dataDir,
		pendingTransfers: make(map[string]*PendingTransfer),
	}

	app.trayManager = NewTrayManager(app, iconData)
	return app
}

// startup вызывается при старте приложения
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Init clipboard
	if err := clipboard.Init(); err != nil {
		log.Printf("[App] Failed to init clipboard: %v", err)
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
	a.messenger.SetNickname(nickname)
	a.messenger.SetProfileUpdateHandler(a.onProfileUpdate)
	a.messenger.SetProfileRequestHandler(a.onProfileRequest)
	a.messenger.SetFileOfferHandler(a.onFileOffer)
	a.messenger.SetFileResponseHandler(a.onFileResponse)

	// Запускаем мессенджер
	if err := a.messenger.Start(a.ctx); err != nil {
		log.Printf("[App] Messenger start failed: %v", err)
		a.setNetworkStatus(NetworkStatusError)
		return
	}

	a.setNetworkStatus(NetworkStatusOnline)
	log.Printf("[App] Connected to I2P. Destination: %s...", destination[:32])
}

// saveAttachment сохраняет вложение на диск
func (a *App) saveAttachment(filename string, data []byte) (string, error) {
	if a.identity == nil {
		return "", fmt.Errorf("user not logged in")
	}

	// Создаём директорию для медиа если нет
	mediaDir := filepath.Join(a.dataDir, "users", a.identity.Keys.UserID, "media")
	if err := os.MkdirAll(mediaDir, 0700); err != nil {
		return "", err
	}

	// Генерируем уникальное имя файла для предотвращения коллизий
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

// SendFileMessage отправляет сообщение с файлами
func (a *App) SendFileMessage(chatID, text string, files []string, isRaw bool) error {
	if a.messenger == nil {
		return fmt.Errorf("messenger not started")
	}

	// Ищем I2P адрес контакта по chatID
	// Ищем контакт по ID (первый аргумент это contactID, как и в SendText)
	contact, err := a.repo.GetContact(a.ctx, chatID)
	if err != nil {
		return fmt.Errorf("contact not found: %w", err)
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	destination := contact.I2PAddress
	actualChatID := contact.ChatID
	if actualChatID == "" {
		// Calculate if missing (shouldn't happen for active chat but good safety)
		actualChatID = identity.CalculateChatID(a.identity.Keys.PublicKeyBase64, contact.PublicKey)
	}

	attachments := make([]*pb.Attachment, 0, len(files))

	for _, filePath := range files {
		// Обрабатываем файл
		var data []byte
		var mimeType string
		var width, height int
		var isCompressed bool

		// Check if it's an image for compression
		ext := strings.ToLower(filepath.Ext(filePath))
		isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"

		if isRaw || !isImage {
			// Читаем как есть
			d, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", filePath, err)
			}
			data = d
			mimeType = mime.TypeByExtension(ext)
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			isCompressed = false
			if isImage {
				wd, ht, _ := utils.GetImageDimensions(filePath)
				width, height = wd, ht
			}
		} else {
			// Алиас: isRaw == false AND isImage == true => Compress
			// Сжимаем
			// Макс размер 1280x1280
			d, mime, w, h, err := utils.CompressImage(filePath, 1280, 1280)
			if err != nil {
				return fmt.Errorf("failed to compress image %s: %w", filePath, err)
			}
			data = d
			mimeType = mime
			width, height = w, h
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

	// Отправляем через мессенджер
	if isRaw {
		// Для uncompressed отправляем сначала предложение (Offer)
		// Генерируем ID сообщения заранее
		now := time.Now().UnixMilli()
		msgID := fmt.Sprintf("%d-%s", now, a.identity.Keys.UserID[:8])

		// Сохраняем информацию о передаче
		a.transferMu.Lock()
		a.pendingTransfers[msgID] = &PendingTransfer{
			Destination: destination,
			ChatID:      actualChatID,
			Files:       files, // Raw local paths
			MessageID:   msgID,
			Timestamp:   now,
		}
		a.transferMu.Unlock()

		// Считаем общий размер
		var totalSize int64
		for _, f := range files {
			info, _ := os.Stat(f)
			if info != nil {
				totalSize += info.Size()
			}
		}

		filenames := make([]string, len(files))
		for i, f := range files {
			filenames[i] = filepath.Base(f)
		}

		// Отправляем Offer
		if err := a.messenger.SendFileOffer(destination, actualChatID, msgID, filenames, totalSize, int32(len(files))); err != nil {
			return fmt.Errorf("failed to send file offer: %w", err)
		}

		// Сохраняем сообщение локально со статусом 'offered' (или 'sending_offer')
		// Пока используем стандартный статус, но можно добавить custom поле в Content
		// Или просто статус PENDING.
		// Используем core.MessageStatusPending если есть, или Sent.
		// В UI мы будем отображать "Ожидание подтверждения" если это RAW.

		msg := &core.Message{
			ID:          msgID,
			ChatID:      actualChatID,
			SenderID:    a.identity.Keys.PublicKeyBase64,
			Content:     text,         // Может быть пустым
			ContentType: "file_offer", // Специальный тип
			Status:      core.MessageStatusSent,
			IsOutgoing:  true,
			Timestamp:   now,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			// Attachments мы пока НЕ сохраняем в БД как attachments,
			// так как они еще не отправлены. Но нам нужно их отображать.
			// Сохраним их как локальные вложения?
			// Если мы их сохраним, то UI может попытаться их показать.
			// Давайте сохраним, но пометим как-то?
			// Пока сохраняем как есть.
		}

		// Save attachments info to DB so we show them in UI
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
				MimeType:     "application/octet-stream", // Raw
				Size:         size,
				LocalPath:    f,
				IsCompressed: false,
			}
			coreAttachments = append(coreAttachments, coreAtt)
		}
		msg.Attachments = coreAttachments

		if err := a.repo.SaveMessage(a.ctx, msg); err != nil {
			log.Printf("[App] Failed to save offer message: %v", err)
		}

		// Emit event update
		runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
			"id":          msg.ID,
			"chatId":      msg.ChatID,
			"senderId":    msg.SenderID,
			"content":     msg.Content,
			"timestamp":   msg.Timestamp,
			"isOutgoing":  msg.IsOutgoing,
			"contentType": "file_offer",
		})

		return nil
	}

	// Normal (compressed) flow
	if err := a.messenger.SendAttachmentMessage(destination, actualChatID, text, attachments); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Сохраняем сообщение локально
	// Нам нужно сохранить и сами файлы локально, чтобы они отображались в чате
	// Мы можем использовать saveAttachment для этого
	coreAttachments := make([]*core.Attachment, 0, len(attachments))
	for _, att := range attachments {
		savedPath, err := a.saveAttachment(att.Filename, att.Data)
		if err != nil {
			log.Printf("[App] Failed to save sent attachment locally: %v", err)
			continue
		}
		coreAtt := &core.Attachment{
			ID: att.Id,
			// MessageID будет присвоен при сохранении сообщения
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

	// Temporarily: I will use the same ID generation logic.
	now := time.Now().UnixMilli()
	msgID := fmt.Sprintf("%d-%s", now, a.identity.Keys.UserID[:8])

	msg := &core.Message{
		ID:          msgID,
		ChatID:      actualChatID,
		SenderID:    a.identity.Keys.PublicKeyBase64,
		Content:     text,
		ContentType: "mixed", // or text if text is present
		Status:      core.MessageStatusSent,
		IsOutgoing:  true,
		Timestamp:   now,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Attachments: coreAttachments,
	}

	if err := a.repo.SaveMessage(a.ctx, msg); err != nil {
		log.Printf("[App] Failed to save sent message: %v", err)
	}

	// Prepare attachments for event
	attList := make([]map[string]interface{}, 0, len(msg.Attachments))
	for _, att := range coreAttachments {
		attList = append(attList, map[string]interface{}{
			"id":         att.ID,
			"filename":   att.Filename,
			"mimeType":   att.MimeType,
			"size":       att.Size,
			"local_path": att.LocalPath,
		})
	}

	// Emit event update
	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"id":          msg.ID,
		"chatId":      msg.ChatID,
		"senderId":    msg.SenderID,
		"content":     msg.Content,
		"timestamp":   msg.Timestamp,
		"isOutgoing":  msg.IsOutgoing,
		"contentType": msg.ContentType,
		"attachments": attList,
		"status":      "sent",
	})

	return nil
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

	// Подготавливаем вложения для события
	attList := make([]map[string]interface{}, 0, len(msg.Attachments))
	for _, att := range msg.Attachments {
		attList = append(attList, map[string]interface{}{
			"id":         att.ID,
			"filename":   att.Filename,
			"mimeType":   att.MimeType,
			"size":       att.Size,
			"local_path": att.LocalPath,
		})
	}

	// Отправляем событие во фронтенд
	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"id":          msg.ID,
		"chatId":      msg.ChatID,
		"senderId":    msg.SenderID,
		"content":     msg.Content, // Может быть пустым для файлов
		"timestamp":   msg.Timestamp,
		"isOutgoing":  msg.IsOutgoing,
		"contentType": msg.ContentType,
		"attachments": attList,
	})

	log.Printf("[App] New message received: %s (attachments: %d)", msg.Content, len(msg.Attachments))
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
	// Читаем буфер обмена через Wails runtime
	data, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("clipboard is empty")
	}

	destination := strings.TrimSpace(data)

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

	// Отправляем событие во фронтенд
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
	runtime.ClipboardSetText(a.ctx, text)
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

// GetFileBase64 читает файл и возвращает base64 строку
func (a *App) GetFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// SaveTempImage сохраняет base64 изображение во временный файл
func (a *App) SaveTempImage(base64Data string, name string) (string, error) {
	// Remove data URI prefix if present
	parts := strings.Split(base64Data, ",")
	if len(parts) > 1 {
		base64Data = parts[1]
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Create temp directory if not exists
	tempDir := filepath.Join(os.TempDir(), "teleghost_uploads")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Create unique filename
	filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), name)
	path := filepath.Join(tempDir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return path, nil
}

// SelectFiles открывает диалог выбора файлов
func (a *App) SelectFiles() ([]string, error) {
	selection, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите файлы",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "All Files",
				Pattern:     "*",
			},
			{
				DisplayName: "Images",
				Pattern:     "*.jpg;*.jpeg;*.png;*.webp;*.gif;*.bmp",
			},
		},
	})
	return selection, err
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

	packet := &pb.Packet{
		Type: pb.PacketType_PROFILE_UPDATE,
		Payload: func() []byte {
			update := &pb.ProfileUpdate{
				Nickname: profile.Nickname,
				Bio:      "", // TODO: Add Bio to user profile struct
				Avatar:   avatarBytes,
			}
			data, _ := proto.Marshal(update)
			return data
		}(),
	}

	a.messenger.Broadcast(packet)
}

func (a *App) onProfileUpdate(pubKey, nickname, bio string, avatar []byte) {
	if a.repo == nil {
		return
	}

	contact, err := a.repo.GetContactByPublicKey(a.ctx, pubKey)
	if err != nil || contact == nil {
		log.Printf("[App] Received profile update from unknown contact: %s", pubKey[:16])
		return
	}

	contact.Nickname = nickname
	// contact.Bio = bio // Если в модели Contact есть поле Bio
	// contact.Avatar = string(avatar) // Если хотим сохранять аватар. Лучше base64.
	if len(avatar) > 0 {
		contact.Avatar = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(avatar)
	}

	contact.UpdatedAt = time.Now()
	a.repo.SaveContact(a.ctx, contact)

	log.Printf("[App] Updated profile for %s", nickname)
	runtime.EventsEmit(a.ctx, "contact_updated", contact.ID)
}

// RequestProfileUpdate запрашивает обновление профиля у контакта
func (a *App) RequestProfileUpdate(contactID string) error {
	if a.messenger == nil {
		return fmt.Errorf("not connected to I2P")
	}

	contact, err := a.repo.GetContact(a.ctx, contactID)
	if err != nil {
		return err
	}
	if contact == nil {
		return fmt.Errorf("contact not found")
	}

	if err := a.messenger.SendProfileRequest(contact.I2PAddress); err != nil {
		return err
	}

	return nil
}

// onProfileRequest обрабатывает входящий запрос профиля
func (a *App) onProfileRequest(requestorPubKey string) {
	log.Printf("[App] Sending profile update to %s", requestorPubKey[:16])

	// В данном случае мы можем отправить обновление конкретному пиру.
	// Но у нас пока есть только Broadcast или SendMessage по адресу.
	// Адрес мы не знаем из pubKey напрямую, но он должен быть в соединениях messenger или в контактах.
	// Messenger знает соединение.
	// Для простоты, мы можем сделать broadcast (не очень эффективно) или добавить SendProfileUpdateTo(pubKey).
	// Сейчас сделаем broadcast, так как это проще :)
	// TODO: Оптимизировать отправку конкретному пиру
	a.broadcastProfileUpdate()
}

// CopyImageToClipboard copies image from path to clipboard
func (a *App) CopyImageToClipboard(path string) error {
	// Read file
	fileData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Decode to image.Image to check format and convert if needed
	img, _, err := image.Decode(bytes.NewReader(fileData))
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Encode to PNG buffer (clipboard.FmtImage usually expects PNG)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("failed to encode to png: %w", err)
	}

	// Write to clipboard
	clipboard.Write(clipboard.FmtImage, buf.Bytes())
	return nil
}

// GetImageThumbnail создает уменьшенную копию изображения и возвращает base64
func (a *App) GetImageThumbnail(path string, width, height uint) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", err
	}

	// Bilinear resampling for speed
	m := resize.Thumbnail(width, height, img, resize.Bilinear)

	var buf bytes.Buffer
	err = png.Encode(&buf, m)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// onFileOffer handles incoming file transfer offers
func (a *App) onFileOffer(senderPubKey, messageID, chatID string, filenames []string, totalSize int64, fileCount int32) {
	if a.repo == nil {
		return
	}

	// Create a message representing the offer
	// We need to resolve sender ID and Chat ID properly
	// senderPubKey is the key.
	// chatID from packet might be empty or we should verify it.
	// In Messenger.SendFileOffer we send chatID.
	// But we should probably look up contact by senderPubKey to be sure.

	contact, err := a.repo.GetContactByPublicKey(a.ctx, senderPubKey)
	if err != nil || contact == nil {
		log.Printf("[App] Received file offer from unknown contact: %s", senderPubKey[:16])
		// Optionally create unknown contact logic here, similar to onMessageReceived
		return
	}

	// Save this offer in pendingTransfers (Incoming)
	// We use the same map, but maybe we need to distinguish?
	// For incoming, we need to know who sent it to send response back.
	// Destination (for response) is the sender's I2P address.
	a.transferMu.Lock()
	a.pendingTransfers[messageID] = &PendingTransfer{
		Destination: contact.I2PAddress, // Send response here
		ChatID:      contact.ChatID,
		Files:       filenames, // Only names available now
		MessageID:   messageID,
		Timestamp:   time.Now().UnixMilli(),
	}
	a.transferMu.Unlock()

	// Save message to DB with 'file_offer' type
	msg := &core.Message{
		ID:          messageID,
		ChatID:      contact.ChatID,
		SenderID:    senderPubKey,
		Content:     "", // Empty content or description
		ContentType: "file_offer",
		Status:      core.MessageStatusDelivered, // It is delivered info
		IsOutgoing:  false,
		Timestamp:   time.Now().UnixMilli(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Save attachments info (placeholders)
	coreAttachments := make([]*core.Attachment, 0, len(filenames))
	for _, fname := range filenames {
		coreAtt := &core.Attachment{
			ID:           uuid.New().String(),
			MessageID:    messageID,
			Filename:     fname,
			MimeType:     "application/octet-stream",
			Size:         totalSize / int64(fileCount), // Approximation if not per file
			LocalPath:    "",                           // Not downloaded yet
			IsCompressed: false,
		}
		coreAttachments = append(coreAttachments, coreAtt)
	}
	msg.Attachments = coreAttachments

	if err := a.repo.SaveMessage(a.ctx, msg); err != nil {
		log.Printf("[App] Failed to save incoming offer: %v", err)
	}

	// Notify Frontend
	runtime.EventsEmit(a.ctx, "new_message", map[string]interface{}{
		"id":          msg.ID,
		"chatId":      msg.ChatID,
		"senderId":    msg.SenderID,
		"content":     msg.Content,
		"timestamp":   msg.Timestamp,
		"isOutgoing":  msg.IsOutgoing,
		"contentType": "file_offer",
		"fileCount":   fileCount,
		"totalSize":   totalSize,
		"filenames":   filenames,
	})

	log.Printf("[App] Received file offer from %s: %d files", contact.Nickname, fileCount)
}

// onFileResponse handles response to our file offer
func (a *App) onFileResponse(senderPubKey, messageID, chatID string, accepted bool) {
	log.Printf("[App] Received file response for %s: accepted=%v", messageID[:8], accepted)

	a.transferMu.Lock()
	transfer, exists := a.pendingTransfers[messageID]
	// Remove from pending only if rejected or finished?
	// If accepted, we start sending. We can keep it or remove it if we have all info.
	// We need the file paths to send.
	if !exists {
		a.transferMu.Unlock()
		log.Printf("[App] Transfer info not found for %s", messageID[:8])
		return
	}
	// Warning: we should not remove it yet if we need it for sending.
	// But after sending we should.
	// Let's remove it after sending.
	a.transferMu.Unlock()

	// Update message status in DB
	msg, err := a.repo.GetMessage(a.ctx, messageID)
	if err == nil && msg != nil {
		if accepted {
			// Update status? Or just start sending.
			// Frontend will update based on events.
		} else {
			msg.Status = core.MessageStatusFailed // Or "rejected"
			a.repo.SaveMessage(a.ctx, msg)        // Update status

			// Notify frontend
			runtime.EventsEmit(a.ctx, "message_update", map[string]interface{}{
				"id":     messageID,
				"chatId": msg.ChatID,
				"status": "rejected",
			})

			// Cleanup
			a.transferMu.Lock()
			delete(a.pendingTransfers, messageID)
			a.transferMu.Unlock()
			return
		}
	}

	if accepted {
		// Start sending the actual files!
		// We use the stored file paths in `transfer.Files`
		log.Printf("[App] Starting transfer for %s...", messageID[:8])

		// SendAttachmentMessageWithID logic
		// We need to re-read files.
		attachments := make([]*pb.Attachment, 0, len(transfer.Files))
		for _, filePath := range transfer.Files {
			data, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("[App] Failed to read file %s: %v", filePath, err)
				continue
			}

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

		// Use the SAME MessageID
		if err := a.messenger.SendAttachmentMessageWithID(transfer.Destination, transfer.ChatID, messageID, "", attachments); err != nil {
			log.Printf("[App] Failed to send attachments: %v", err)
			// Update status to failed
		} else {
			log.Printf("[App] Files sent successfully for %s", messageID[:8])
			// Update status in DB
			/*
				if msg != nil {
					msg.Status = core.MessageStatusSent
					msg.ContentType = "mixed" // Now it has content
					a.repo.SaveMessage(a.ctx, msg)
				}
			*/
			// Actually SendAttachmentMessageWithID sends a TextMessage packet.
			// The receiver will handle it as a new message?
			// PROB: Receiver `handleTextMessage` creates a NEW message with `msg.ID`.
			// Since `msg.ID` matches the `FileOffer` message ID?
			// Yes, if we reuse ID.
			// Receiver `handleTextMessage` does: `msg := &core.Message{ ID: textMsg.MessageId ... }`
			// `repo.SaveMessage` usually does `INSERT OR REPLACE`?
			// SQLite `SaveMessage` implementation check:
			// `INSERT OR REPLACE INTO messages ...`
			// So it should overwrite the "Offer" message with the "Real" message.
			// This is perfect! The "Offer" bubble will be replaced by the actual files.
		}

		// Cleanup
		a.transferMu.Lock()
		delete(a.pendingTransfers, messageID)
		a.transferMu.Unlock()
	}
}

// AcceptFileTransfer accepts an incoming file offer
func (a *App) AcceptFileTransfer(messageID string) error {
	a.transferMu.RLock()
	transfer, exists := a.pendingTransfers[messageID]
	a.transferMu.RUnlock()

	if !exists {
		return fmt.Errorf("transfer not found or expired")
	}

	// Send acceptance
	if err := a.messenger.SendFileResponse(transfer.Destination, transfer.ChatID, messageID, true); err != nil {
		return err
	}

	return nil
}

// DeclineFileTransfer declines an incoming file offer
func (a *App) DeclineFileTransfer(messageID string) error {
	a.transferMu.RLock()
	transfer, exists := a.pendingTransfers[messageID]
	a.transferMu.RUnlock()

	if !exists {
		return fmt.Errorf("transfer not found or expired")
	}

	// Send rejection
	if err := a.messenger.SendFileResponse(transfer.Destination, transfer.ChatID, messageID, false); err != nil {
		return err
	}

	// Cleanup
	a.transferMu.Lock()
	delete(a.pendingTransfers, messageID)
	a.transferMu.Unlock()

	// Update local message status
	msg, err := a.repo.GetMessage(a.ctx, messageID)
	if err == nil && msg != nil {
		msg.Status = core.MessageStatusFailed // "rejected"
		a.repo.SaveMessage(a.ctx, msg)
	}

	return nil
}
