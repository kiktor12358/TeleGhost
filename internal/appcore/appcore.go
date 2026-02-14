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
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	// ShareFile shares a file using system share sheet (Mobile) or opens file location (Desktop)
	ShareFile(path string) error
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
	_ = os.MkdirAll(avatarsDir, 0700)

	fullPath := filepath.Join(avatarsDir, filename)
	if err := os.WriteFile(fullPath, data, 0600); err != nil {
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
	FileCount    int                      `json:"FileCount,omitempty"`
	TotalSize    int64                    `json:"TotalSize,omitempty"`
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
	if err := os.MkdirAll(a.DataDir, 0700); err != nil {
		return fmt.Errorf("failed to create DataDir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(a.DataDir, "users"), 0700); err != nil {
		return fmt.Errorf("failed to create users dir: %w", err)
	}

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
		_ = a.Messenger.Stop()
	}
	if a.Router != nil {
		_ = a.Router.Stop()
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
				newPath, saveErr := a.SaveAvatar("my_avatar.png", data)
				if saveErr == nil {
					avatarPath = newPath
					// Также обновляем в БД путь на локальный, а не base64
					if errUpdate := a.Repo.UpdateMyProfile(a.Ctx, nickname, bio, avatarPath); errUpdate != nil {
						log.Printf("[AppCore] Failed to update profile with local avatar path: %v", errUpdate)
					}
				}
			} else {
				log.Printf("[AppCore] Failed to decode base64 avatar: %v", err)
			}
		}

		meta, err := a.ProfileManager.GetProfileByUserID(a.Identity.Keys.UserID)
		if err == nil && meta != nil {
			if err := a.ProfileManager.UpdateProfile(meta.ID, nickname, avatarPath, false, meta.UsePin, "", a.Identity.Mnemonic); err != nil {
				log.Printf("[AppCore] Failed to sync profile with PM: %v", err)
			}
		}
	}

	return nil
}

// ─── Export/Import Account ──────────────────────────────────────────────────

// ExportAccount creates a ZIP archive with the current user's profile and data.
func (a *AppCore) ExportAccount() (string, error) {
	if a.Identity == nil {
		return "", fmt.Errorf("not logged in")
	}
	if a.ProfileManager == nil {
		return "", fmt.Errorf("profile manager not initialized")
	}

	// 1. Find profile metadata
	profileMeta, err := a.ProfileManager.GetProfileByUserID(a.Identity.Keys.UserID)
	if err != nil || profileMeta == nil {
		return "", fmt.Errorf("profile metadata not found: %w", err)
	}

	// 2. Prepare temp zip file
	tempDir := filepath.Join(a.DataDir, "temp")
	if errMkdir := os.MkdirAll(tempDir, 0700); errMkdir != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", errMkdir)
	}
	zipName := fmt.Sprintf("teleghost_export_%s_%d.zip", profileMeta.DisplayName, time.Now().Unix())
	zipPath := filepath.Join(tempDir, zipName)

	outFile, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	defer func() {
		if errClose := w.Close(); errClose != nil {
			log.Printf("[AppCore] Failed to close zip writer: %v", errClose)
		}
	}()

	// 3. Add profile JSON file
	profileJsonPath := filepath.Join(a.DataDir, "profiles", profileMeta.ID+".json")
	if errZip := addFileToZip(w, profileJsonPath, "profile.json"); errZip != nil {
		return "", fmt.Errorf("failed to add profile json: %w", errZip)
	}

	// 4. Add Avatar if separate (it might be referenced in JSON)
	if profileMeta.AvatarPath != "" {
		avatarFullPath := filepath.Join(a.DataDir, "profiles", profileMeta.AvatarPath)
		if _, errStat := os.Stat(avatarFullPath); errStat == nil {
			if errZip := addFileToZip(w, avatarFullPath, profileMeta.AvatarPath); errZip != nil {
				log.Printf("Failed to add avatar to zip: %v", errZip)
			}
		}
	}

	// 5. Add User Data Directory
	userDir := filepath.Join(a.DataDir, "users", a.Identity.Keys.UserID)
	err = filepath.Walk(userDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(userDir, path)
		if err != nil {
			return err
		}

		zipEntryPath := filepath.Join("user_data", relPath)
		return addFileToZip(w, path, zipEntryPath)
	})
	if err != nil {
		return "", fmt.Errorf("failed to add user data: %w", err)
	}

	return zipPath, nil
}

// ImportAccount imports an account from a ZIP file.
func (a *AppCore) ImportAccount(zipPath string) error {
	if a.ProfileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	// 1. Read profile.json from zip
	var profileData []byte
	for _, f := range r.File {
		if f.Name == "profile.json" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			profileData, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			break
		}
	}

	if profileData == nil {
		return fmt.Errorf("invalid archive: profile.json not found")
	}

	// Parse basic info via generic map to avoid struct mismatch if possible, or just struct
	var meta struct {
		ID          string `json:"id"`
		UserID      string `json:"user_id"`
		DisplayName string `json:"display_name"`
		AvatarPath  string `json:"avatar_path"`
	}
	if err := json.Unmarshal(profileData, &meta); err != nil {
		return fmt.Errorf("invalid profile json: %w", err)
	}

	// Check if already exists
	existing, _ := a.ProfileManager.GetProfileByUserID(meta.UserID)
	if existing != nil {
		return fmt.Errorf("account already exists: %s", existing.DisplayName)
	}

	// 2. Restore Profile
	profileDest := filepath.Join(a.DataDir, "profiles", meta.ID+".json")
	if err := os.WriteFile(profileDest, profileData, 0600); err != nil {
		return fmt.Errorf("failed to write profile json: %w", err)
	}

	// 3. Restore files
	for _, f := range r.File {
		if f.Name == "profile.json" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		// Защита от Zip Slip
		if strings.Contains(f.Name, "..") {
			rc.Close()
			continue
		}

		var destPath string
		if strings.HasPrefix(f.Name, "user_data/") {
			rel := strings.TrimPrefix(f.Name, "user_data/")
			destPath = filepath.Join(a.DataDir, "users", meta.UserID, rel)
		} else if f.Name == meta.AvatarPath {
			destPath = filepath.Join(a.DataDir, "profiles", f.Name)
		} else {
			_ = rc.Close()
			continue
		}

		_ = os.MkdirAll(filepath.Dir(destPath), 0700)

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}
		// Ограничиваем размер (защита от decompression bomb) - макс 100 МБ на файл
		if _, err = io.Copy(outFile, io.LimitReader(rc, 100*1024*1024)); err != nil {
			_ = outFile.Close()
			_ = rc.Close()
			return err
		}
		_ = outFile.Close()
		_ = rc.Close()
	}

	log.Printf("Imported account: %s (%s)", meta.DisplayName, meta.ID)
	return nil
}

// Helper to add file to zip
func addFileToZip(w *zip.Writer, srcPath, zipPath string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	writer, err := w.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
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
	_ = os.MkdirAll(userDir, 0700)

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
				_ = os.Remove(keysPath)
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
					if err := a.Repo.SaveUser(a.Ctx, user); err != nil {
						log.Printf("[AppCore] Failed to save I2P destination to DB: %v", err)
					}
				}
				_ = os.Remove(keysPath)
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
func (a *AppCore) onProfileUpdate(senderPubKey, nickname, bio string, avatar []byte, senderAddr string) {
	if a.Repo == nil {
		return
	}

	if len(avatar) > MaxAvatarSize {
		log.Printf("[AppCore] Ignored large avatar (%d bytes) from %s", len(avatar), senderPubKey[:8])
		avatar = nil // Не сохраняем слишком большую аватарку
	}

	contact, _ := a.Repo.GetContactByPublicKey(a.Ctx, senderPubKey)
	if contact == nil {
		// Try to find by address (important for b32-only contacts discovery)
		contact, _ = a.Repo.GetContactByAddress(a.Ctx, senderAddr)
		if contact != nil {
			oldChatID := contact.ChatID
			newChatID := identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, senderPubKey)

			log.Printf("[AppCore] Discovered PublicKey via ProfileUpdate for %s (%s). Migrating ChatID: %s -> %s", contact.Nickname, senderAddr, oldChatID, newChatID)

			contact.PublicKey = senderPubKey
			contact.ChatID = newChatID
			contact.UpdatedAt = time.Now()

			if err := a.Repo.UpdateContactAndMigrateChatID(a.Ctx, contact, oldChatID, newChatID); err != nil {
				log.Printf("[AppCore] Failed to migrate ChatID via ProfileUpdate for %s: %v", contact.Nickname, err)
			}
		} else {
			// Not a contact we know, just return
			return
		}
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
	if err := a.Repo.SaveContact(a.Ctx, contact); err != nil {
		log.Printf("[AppCore] Failed to save contact on profile update: %v", err)
	}
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
		if err := a.Messenger.SendProfileUpdate(contact.I2PAddress, user.Nickname, user.Bio, avatarData); err != nil {
			log.Printf("[AppCore] Failed to send profile update: %v", err)
		}
	}
}

// RequestProfile запрашивает обновление профиля у контакта
func (a *AppCore) RequestProfile(address string) error {
	if a.Messenger == nil {
		return fmt.Errorf("messenger not initialized")
	}
	return a.Messenger.SendProfileRequest(address)
}

// ─── Обработчики входящих сообщений ─────────────────────────────────────────

// OnMessageReceived — обработчик входящих сообщений.
// Полная логика: сохранение в БД, автосоздание контактов, события фронтенду.
func (a *AppCore) OnMessageReceived(msg *core.Message, senderPubKey, senderAddr string) {
	if a.Repo == nil {
		return
	}

	msg.SenderAddr = senderAddr
	var contact *core.Contact
	contact, _ = a.Repo.GetContactByPublicKey(a.Ctx, senderPubKey)
	if contact == nil {
		// Try to find by address (for manual b32 contacts)
		contact, _ = a.Repo.GetContactByAddress(a.Ctx, senderAddr)
		if contact != nil {
			oldChatID := contact.ChatID
			newChatID := identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, senderPubKey)

			log.Printf("[AppCore] Discovered PublicKey for %s (%s). Migrating ChatID: %s -> %s", contact.Nickname, senderAddr, oldChatID, newChatID)

			contact.PublicKey = senderPubKey
			contact.ChatID = newChatID
			contact.UpdatedAt = time.Now()

			if err := a.Repo.UpdateContactAndMigrateChatID(a.Ctx, contact, oldChatID, newChatID); err != nil {
				log.Printf("[AppCore] Failed to migrate ChatID for %s: %v", contact.Nickname, err)
			}
			a.Emitter.Emit("contact_updated")
		} else {
			// Create new contact
			newChatID := identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, senderPubKey)
			contact = &core.Contact{
				ID:         uuid.New().String(),
				PublicKey:  senderPubKey,
				Nickname:   "Unknown " + senderPubKey[:8],
				I2PAddress: senderAddr,
				ChatID:     newChatID,
				AddedAt:    time.Now(),
			}
			if err := a.Repo.SaveContact(a.Ctx, contact); err != nil {
				log.Printf("[AppCore] Failed to auto-save contact: %v", err)
			}
			a.Emitter.Emit("contact_updated")
			// Запрашиваем профиль у нового контакта
			if msg.ContentType == "text" && !contact.IsVerified {
				go func(addr string) {
					if a.Messenger == nil {
						log.Printf("[AppCore] Messenger not initialized, cannot send auto profile request")
						return
					}
					if err := a.Messenger.SendProfileRequest(addr); err != nil {
						log.Printf("[AppCore] Failed to send auto profile request: %v", err)
					}
				}(senderAddr)
			}
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
		"FileCount":    msg.FileCount,
		"TotalSize":    msg.TotalSize,
	})

	if !msg.IsOutgoing {
		// Помечаем как прочитанное сразу, если чат активен
		if a.ActiveChatID == msg.ChatID && a.IsFocused {
			if err := a.Repo.MarkChatAsRead(a.Ctx, msg.ChatID); err != nil {
				log.Printf("[AppCore] Failed to mark chat as read: %v", err)
			}
		}

		// Подавляем уведомление, если приложение видимо, в фокусе и открыт именно этот чат
		if !a.IsVisible || !(a.IsFocused && a.ActiveChatID == msg.ChatID) {
			go a.SendNotification(contact.Nickname, msg.Content, msg.ContentType)
		}
		go a.UpdateUnreadCount()
	}
}

// OnContactRequest — обработчик запросов дружбы.
// OnContactRequest — обработчик запросов дружбы.
func (a *AppCore) OnContactRequest(pubKey, nickname, i2pAddress string) {
	log.Printf("[AppCore] OnContactRequest from %s (%s)", nickname, pubKey[:8])

	if a.Repo == nil {
		return
	}

	// 1. Check if we already have this contact (by address)
	contact, _ := a.Repo.GetContactByAddress(a.Ctx, i2pAddress)
	if contact != nil {
		// Contact exists.
		oldChatID := contact.ChatID
		updated := false
		publicKeyChanged := false

		// Update Public Key if changed
		if contact.PublicKey != pubKey {
			contact.PublicKey = pubKey
			updated = true
			publicKeyChanged = true
		}

		// Update Nickname if meaningful change
		if nickname != "" && nickname != "Unknown" && contact.Nickname != nickname {
			contact.Nickname = nickname
			updated = true
		}

		// Check ChatID change
		newChatID := identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, pubKey)
		if contact.ChatID != newChatID {
			log.Printf("[AppCore] Updating contact %s. ChatID migration: %s -> %s", contact.Nickname, oldChatID, newChatID)
			contact.ChatID = newChatID
			updated = true
		}

		if updated {
			contact.UpdatedAt = time.Now()
			// Save Contact AND Migrate Messages in one transaction
			if err := a.Repo.UpdateContactAndMigrateChatID(a.Ctx, contact, oldChatID, newChatID); err != nil {
				log.Printf("[AppCore] Failed to update contact and migrate messages: %v", err)
				return
			}
			a.Emitter.Emit("contact_updated")

			// Send handshake back if public key was updated (to ensure they have ours)
			// But avoid infinite loop if key didn't change (handled by 'updated' flag logic which checks contact.PublicKey != pubKey)
			if publicKeyChanged && a.Messenger != nil {
				// We just updated it to pubKey, so checking == is always true here.
				// The guard is that we only enter this block if it was DIFFERENT before.
				go func(addr string) {
					if errHandshake := a.Messenger.SendHandshake(addr); errHandshake != nil {
						log.Printf("[AppCore] Failed to send handshake: %v", errHandshake)
					}
				}(contact.I2PAddress)
			}
		}
	} else {
		// New contact
		newChatID := identity.CalculateChatID(a.Identity.Keys.PublicKeyBase64, pubKey)
		contact = &core.Contact{
			ID:         uuid.New().String(),
			PublicKey:  pubKey,
			Nickname:   nickname,
			I2PAddress: i2pAddress,
			ChatID:     newChatID,
			AddedAt:    time.Now(),
		}
		if err := a.Repo.SaveContact(a.Ctx, contact); err == nil {
			a.Emitter.Emit("new_contact", map[string]interface{}{
				"nickname": nickname,
			})
			a.Emitter.Emit("contact_updated")

			// Send handshake back to new contact so they get our key
			if a.Messenger != nil {
				go func(addr string) {
					if errH := a.Messenger.SendHandshake(addr); errH != nil {
						log.Printf("[AppCore] Failed to send handshake: %v", errH)
					}
				}(contact.I2PAddress)
				// Also request profile
				go func(addr string) {
					if errP := a.Messenger.SendProfileRequest(addr); errP != nil {
						log.Printf("[AppCore] Failed to send profile request: %v", errP)
					}
				}(i2pAddress)
			}
		} else {
			log.Printf("[AppCore] Failed to save new contact: %v", err)
		}
	}
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
	if len([]rune(author)) > 50 {
		author = string([]rune(author)[:47]) + "..."
	}

	content := orig.Content
	runes := []rune(content)
	if len(runes) > 100 {
		content = string(runes[:97]) + "..."
	}

	return &ReplyPreview{
		AuthorName: author,
		Content:    content,
	}
}
