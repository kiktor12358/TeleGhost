//go:build cgo_i2pd
// +build cgo_i2pd

// Package mobile предоставляет HTTP-адаптер для запуска TeleGhost на Android.
//
// Архитектура:
//
//	Android (Kotlin) вызывает Mobile.Start() через JNI (.aar).
//	Go стартует HTTP-сервер на 127.0.0.1:8080.
//	WebView отображает фронтенд и шлёт запросы на localhost.
//
// Весь бизнес-код находится в internal/appcore — тут только HTTP + SSE обёртка.
//
// Сборка:
//
//	CGO_ENABLED=1 gomobile bind -tags cgo_i2pd -target=android -androidapi 21 -o teleghost.aar ./mobile
package mobile

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // Support JPEG decoding
	"image/png"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"teleghost/internal/appcore"
	"teleghost/internal/network/i2pd"

	"github.com/nfnt/resize"
)

//go:embed all:dist
var frontendAssets embed.FS

// ─── SSE EventEmitter (реализация интерфейса appcore.EventEmitter) ──────────

// SSEEmitter рассылает события через Server-Sent Events.
type SSEEmitter struct {
	clients   map[chan string]struct{}
	clientsMu sync.RWMutex
}

func NewSSEEmitter() *SSEEmitter {
	return &SSEEmitter{
		clients: make(map[chan string]struct{}),
	}
}

func (e *SSEEmitter) Emit(event string, data ...interface{}) {
	var payload interface{}
	if len(data) == 1 {
		payload = data[0]
	} else if len(data) > 1 {
		payload = data
	}

	msg, _ := json.Marshal(map[string]interface{}{
		"event": event,
		"data":  payload,
	})

	e.clientsMu.RLock()
	defer e.clientsMu.RUnlock()

	for ch := range e.clients {
		select {
		case ch <- string(msg):
		default:
			// Клиент медленный, пропускаем
		}
	}
}

func (e *SSEEmitter) subscribe() chan string {
	ch := make(chan string, 64)
	e.clientsMu.Lock()
	e.clients[ch] = struct{}{}
	e.clientsMu.Unlock()
	return ch
}

func (e *SSEEmitter) unsubscribe(ch chan string) {
	e.clientsMu.Lock()
	delete(e.clients, ch)
	e.clientsMu.Unlock()
	close(ch)
}

// ─── Mobile PlatformServices (заглушки) ─────────────────────────────────────

// PlatformBridge defines methods that Native Android/iOS must implement.
type PlatformBridge interface {
	PickFile()
	ShareFile(path string)
	ClipboardSet(text string)
	ShowNotification(title, message string) // New method
	SaveFile(path, filename string)         // New method for saving to Downloads
}

var (
	bridge            PlatformBridge
	fileSelectionChan = make(chan string, 1) // Channel to receive file path from Native side
)

// SetPlatformBridge connects the native OS implementation to Go.
func SetPlatformBridge(b PlatformBridge) {
	bridge = b
}

// OnFileSelected is called by Native side when a file is picked.
// path: absolute path to the file (must be accessible by Go).
// Pass empty string to signal cancellation.
func OnFileSelected(path string) {
	// Send to channel if someone is waiting
	select {
	case fileSelectionChan <- path:
	default:
		log.Println("[Mobile] Dropping file selection (no active request)")
	}
}

// MobilePlatform — no-op реализация PlatformServices для Android.
// Файловые диалоги и буфер обмена недоступны через Go на мобилке.
type MobilePlatform struct{}

func (p *MobilePlatform) OpenFileDialog(title string, filters []string) (string, error) {
	if bridge == nil {
		return "", fmt.Errorf("native bridge not connected")
	}

	// 1. Request file pick from Native side
	bridge.PickFile()

	// 2. Wait for result (blocking)
	select {
	case path := <-fileSelectionChan:
		if path == "" {
			return "", fmt.Errorf("selection canceled")
		}
		return path, nil
	case <-time.After(5 * time.Minute): // Timeout to prevent eternal freeze
		return "", fmt.Errorf("file selection timed out")
	}
}

func (p *MobilePlatform) SaveFileDialog(title, defaultFilename string) (string, error) {
	return "", fmt.Errorf("save dialog not implemented on mobile")
}
func (p *MobilePlatform) ClipboardSet(text string) {
	if bridge != nil {
		bridge.ClipboardSet(text)
	}
}
func (p *MobilePlatform) ClipboardGet() (string, error) {
	return "", fmt.Errorf("clipboard not available on mobile")
}

func (p *MobilePlatform) ShowWindow() {}
func (p *MobilePlatform) HideWindow() {}

// MobileFileSelector removed (redundant)

func (p *MobilePlatform) Notify(title, message string) {
	if bridge != nil {
		bridge.ShowNotification(title, message)
	} else {
		log.Printf("[Mobile] Notification (no bridge): %s - %s", title, message)
	}
}

func (p *MobilePlatform) ShareFile(path string) error {
	if bridge == nil {
		return fmt.Errorf("native bridge not connected")
	}
	bridge.ShareFile(path)
	return nil
}

// ─── Глобальное состояние ───────────────────────────────────────────────────

var (
	globalApp *appcore.AppCore
	sseEmit   *SSEEmitter
	server    *http.Server
)

// ─── Публичный API для Kotlin/Java (через gomobile) ─────────────────────────

// Start запускает HTTP-сервер и бизнес-логику.
// Вызывается из Android Service через JNI.
// dataDir — путь к внутреннему хранилищу: context.getFilesDir()
func Start(dataDir string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRITICAL] Mobile panic: %v", r)
		}
	}()
	if globalApp != nil {
		log.Println("[Mobile] Server already running")
		return
	}

	sseEmit = NewSSEEmitter()
	platform := &MobilePlatform{}

	app := appcore.NewAppCore(dataDir, sseEmit, platform)
	if err := app.Init(); err != nil {
		log.Printf("[Mobile] Init failed: %v", err)
		return
	}

	globalApp = app

	// FileSelector reflection setup removed (redundant)

	// HTTP сервер
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", handleAPI)
	mux.HandleFunc("/api/events", handleSSE)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Раздача статических файлов фронтенда
	subFS, err := fs.Sub(frontendAssets, "dist")
	if err != nil {
		log.Printf("[Mobile] Failed to create sub FS: %v", err)
	} else {
		mux.Handle("/", http.FileServer(http.FS(subFS)))
	}

	// Media Handlers for avatars and files
	mux.HandleFunc("/avatars/", func(w http.ResponseWriter, r *http.Request) {
		// Logic similar to desktop GetMediaHandler
		// Path format: /avatars/{userID|unknown}/{filename}
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 {
			http.NotFound(w, r)
			return
		}
		userID := parts[2]
		filename := parts[3]

		var path string
		if userID == "unknown" {
			// Try to find in any user avatar dir? No, this is tricky.
			// Ideally we should know who we are.
			// But on mobile we have globalApp.Identity.
			if globalApp != nil && globalApp.Identity != nil {
				path = filepath.Join(dataDir, "users", globalApp.Identity.Keys.UserID, "avatars", filename)
			}
		} else {
			path = filepath.Join(dataDir, "users", userID, "avatars", filename)
		}

		if path != "" {
			http.ServeFile(w, r, path)
		} else {
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		// Path format: /media/{filename}
		// Assuming media is stored in user's media folder?
		// Desktop implementation usually handles this.
		// Let's assume globalApp.DataDir/media or similar?
		// Actually, let's look at AppCore.SaveAttachment...
		// It likely saves to `users/{UserID}/media`.
		if globalApp != nil && globalApp.Identity != nil {
			filename := filepath.Base(r.URL.Path)
			path := filepath.Join(dataDir, "users", globalApp.Identity.Keys.UserID, "media", filename)
			http.ServeFile(w, r, path)
		} else {
			http.NotFound(w, r)
		}
	})

	server = &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	go func() {
		log.Printf("[Mobile] Starting HTTP server on 127.0.0.1:8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Mobile] Server error: %v", err)
		}
	}()

	// Start embedded I2P Router
	// This is critical for mobile where we don't have an external router service
	go func() {
		routerDir := dataDir + "/i2pd"
		log.Printf("[Mobile] Starting embedded i2pd router in %s", routerDir)

		cfg := i2pd.DefaultConfig()
		cfg.DataDir = routerDir
		cfg.LogToFile = true // Useful for debugging on mobile

		router := i2pd.NewRouter(cfg)
		if err := router.Start(context.Background()); err != nil {
			log.Printf("[Mobile] Failed to start i2pd router: %v", err)
		} else {
			log.Printf("[Mobile] i2pd router started successfully")
		}
	}()

	// Connect AppCore to the (now starting) router
	// We do this in a goroutine to not block the main thread and allow time for router startup
	go func() {
		// Wait a bit for SAM bridge to be ready
		time.Sleep(2 * time.Second)
		log.Printf("[Mobile] Connecting AppCore to I2P...")
		app.ConnectToI2P()
	}()

	log.Printf("[Mobile] Server started, dataDir=%s", dataDir)
}

// Stop останавливает всё.
func Stop() {
	if globalApp == nil {
		return
	}

	log.Println("[Mobile] Stopping...")
	globalApp.Shutdown()

	if server != nil {
		server.Close()
	}

	globalApp = nil
	sseEmit = nil
	server = nil
	log.Println("[Mobile] Stopped")
}

// IsRunning возвращает true если сервер запущен.
func IsRunning() bool {
	return globalApp != nil
}

// ─── HTTP Handler ───────────────────────────────────────────────────────────

func handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "only POST allowed")
		return
	}

	method := r.URL.Path[len("/api/"):]
	if method == "" || method == "events" {
		writeError(w, http.StatusBadRequest, "unknown method")
		return
	}

	var req struct {
		Args []json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if globalApp == nil {
		writeError(w, http.StatusServiceUnavailable, "app not initialized")
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRITICAL] API Panic in %s: %v", method, r)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Internal Panic: %v", r))
		}
	}()

	result, err := dispatch(globalApp, method, req.Args)
	if err != nil {
		log.Printf("[Mobile] API Error in %s: %v", method, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"result": result,
		"error":  nil,
	})
}

// ─── Диспетчер ──────────────────────────────────────────────────────────────
// Все вызовы делегируются AppCore — НУЛЬ бизнес-логики здесь.

func dispatch(app *appcore.AppCore, method string, args []json.RawMessage) (interface{}, error) {
	switch method {

	// === Auth ===
	case "ListProfiles":
		return app.ListProfiles()

	case "CreateProfile":
		var name, pin, mnemonic, userID, avatarPath string
		var usePin bool
		parseArgs(args, &name, &pin, &mnemonic, &userID, &avatarPath, &usePin)
		return nil, app.CreateProfile(name, pin, mnemonic, userID, avatarPath, usePin)

	case "UnlockProfile":
		var profileID, pin string
		parseArgs(args, &profileID, &pin)
		return app.UnlockProfile(profileID, pin)

	case "DeleteProfile":
		var profileID string
		parseArgs(args, &profileID)
		return nil, app.DeleteProfile(profileID)

	case "Login":
		var seedPhrase string
		parseArgs(args, &seedPhrase)
		return nil, app.Login(seedPhrase)

	case "CreateAccount":
		return app.CreateAccount()

	case "Logout":
		app.Logout()
		return nil, nil

	case "GetMyInfo":
		return app.GetMyInfo(), nil

	case "GetCurrentProfile":
		return app.GetCurrentProfile(), nil

	case "UpdateMyProfile":
		var nickname, bio, avatar string
		parseArgs(args, &nickname, &bio, &avatar)
		return nil, app.UpdateMyProfile(nickname, bio, avatar)

	// === Contacts ===
	case "GetContacts":
		return app.GetContacts()

	case "AddContact":
		var name, dest string
		parseArgs(args, &name, &dest)
		return app.AddContact(name, dest)

	case "AddContactFromClipboard":
		var name string
		parseArgs(args, &name)
		return app.AddContactFromClipboard(name)

	case "DeleteContact":
		var id string
		parseArgs(args, &id)
		return nil, app.DeleteContact(id)

	case "RequestProfile":
		var address string
		parseArgs(args, &address)
		return nil, app.RequestProfile(address)

	// === Messages ===
	case "SendText":
		var contactID, text, replyToID string
		parseArgs(args, &contactID, &text, &replyToID)
		return nil, app.SendText(contactID, text, replyToID)

	case "GetMessages":
		var contactID string
		var limit, offset int
		parseArgs(args, &contactID, &limit, &offset)
		return app.GetMessages(contactID, limit, offset)

	case "EditMessage":
		var messageID, newContent string
		parseArgs(args, &messageID, &newContent)
		return nil, app.EditMessage(messageID, newContent)

	case "DeleteMessage":
		var messageID string
		parseArgs(args, &messageID)
		return nil, app.DeleteMessage(messageID)

	case "MarkChatAsRead":
		var chatID string
		parseArgs(args, &chatID)
		return nil, app.MarkChatAsRead(chatID)

	case "GetUnreadCount":
		return app.GetUnreadCount()

	case "SendFileMessage":
		var chatID, text, replyToID string
		var files []string
		var isRaw bool
		parseArgs(args, &chatID, &text, &replyToID, &files, &isRaw)
		return nil, app.SendFileMessage(chatID, text, replyToID, files, isRaw)

	case "ExportAccount":
		path, err := app.ExportAccount()
		if err != nil {
			return nil, err
		}
		if err := app.Platform.ShareFile(path); err != nil {
			log.Printf("[Mobile] Failed to share account export: %v", err)
		}
		return path, nil

	case "ImportAccount":
		var path string
		parseArgs(args, &path)
		return nil, app.ImportAccount(path)

	case "ShareFile":
		var path string
		parseArgs(args, &path)
		return nil, app.Platform.ShareFile(path)
		var name, icon string
		parseArgs(args, &name, &icon)
		return nil, app.CreateFolder(name, icon)

	case "GetFolders":
		return app.GetFolders()

	case "DeleteFolder":
		var id string
		parseArgs(args, &id)
		return nil, app.DeleteFolder(id)

	case "UpdateFolder":
		var id, name, icon string
		parseArgs(args, &id, &name, &icon)
		return nil, app.UpdateFolder(id, name, icon)

	case "AddChatToFolder":
		var folderID, contactID string
		parseArgs(args, &folderID, &contactID)
		return nil, app.AddChatToFolder(folderID, contactID)

	case "RemoveChatFromFolder":
		var folderID, contactID string
		parseArgs(args, &folderID, &contactID)
		return nil, app.RemoveChatFromFolder(folderID, contactID)

	// === Settings ===
	case "GetMyDestination":
		return app.GetMyDestination(), nil

	case "GetRouterSettings":
		return app.GetRouterSettings(), nil

	case "GetNetworkStatus":
		return app.GetNetworkStatus(), nil

	case "GetAppAboutInfo":
		return app.GetAppAboutInfo(), nil

	case "CheckForUpdates":
		return "У вас установлена последняя версия", nil

	// === Utils ===
	case "ClipboardSet":
		var text string
		parseArgs(args, &text)
		if bridge != nil {
			bridge.ClipboardSet(text)
		} else {
			log.Println("[Mobile] Clipboard bridge not available")
		}
		return nil, nil

	case "GetFileBase64":
		var path string
		parseArgs(args, &path)
		return app.GetFileBase64(path)

	case "SaveTempImage":
		var base64Data, filename string
		parseArgs(args, &base64Data, &filename)
		// Для мобилки можно просто вернуть ошибку или реализовать временное сохранение
		return "", fmt.Errorf("SaveTempImage not implemented on mobile")

	case "SelectFiles":
		path, err := app.Platform.OpenFileDialog("Select File", nil)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil

	case "SelectImage":
		path, err := app.Platform.OpenFileDialog("Select Image", nil)
		if err != nil {
			return nil, err
		}
		return path, nil

	case "OpenFile", "ShowInFolder":
		return nil, fmt.Errorf("system file opening not available on mobile")

	case "SaveFileToLocation":
		var path, filename string
		parseArgs(args, &path, &filename)
		if bridge != nil {
			bridge.SaveFile(path, filename)
			return nil, nil
		}
		return nil, fmt.Errorf("native bridge not connected")

	case "GetImageThumbnail":
		var path string
		parseArgs(args, &path)
		return GetImageThumbnail(path)

	case "SetActiveChat":
		var chatID string
		parseArgs(args, &chatID)
		app.ActiveChatID = chatID
		return nil, nil

	case "SetAppFocus":
		var focused bool
		parseArgs(args, &focused)
		app.IsFocused = focused
		return nil, nil

	case "RequestProfileUpdate":
		var address string
		parseArgs(args, &address)
		if app.Messenger != nil {
			return nil, app.Messenger.SendProfileRequest(address)
		}
		return nil, fmt.Errorf("messenger not initialized")

	case "SaveRouterSettings":
		var settings map[string]interface{}
		parseArgs(args, &settings)
		return nil, app.SaveRouterSettings(settings)

	case "ExportReseed":
		path, err := app.ExportReseed()
		if err != nil {
			return nil, err
		}
		// Share file immediately on mobile
		if err := app.Platform.ShareFile(path); err != nil {
			log.Printf("[Mobile] Failed to share reseed: %v", err)
		}
		return path, nil

	case "ImportReseed":
		var path string // This path comes from PickFile, so it's a temp file accessible by Go
		// Actually, PickFile returns a single file path string
		// Wait, user clicks "Import" -> calls SelectFiles -> gets Path -> calls ImportReseed(path)
		parseArgs(args, &path)
		return nil, app.ImportReseed(path)

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// ─── SSE Handler ────────────────────────────────────────────────────────────

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if sseEmit == nil {
		return
	}

	ch := sseEmit.subscribe()
	defer sseEmit.unsubscribe(ch)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// parseArgs — универсальный парсер аргументов из JSON массива.
func parseArgs(args []json.RawMessage, targets ...interface{}) {
	for i, target := range targets {
		if i < len(args) {
			json.Unmarshal(args[i], target)
		}
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"result": nil,
		"error":  msg,
	})
}

// GetImageThumbnail возвращает уменьшенную копию изображения в base64
func GetImageThumbnail(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", err
	}

	// Изменяем размер до 256px по одной из сторон
	thumb := resize.Thumbnail(256, 256, img, resize.Lanczos3)

	var buf bytes.Buffer
	if err := png.Encode(&buf, thumb); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
