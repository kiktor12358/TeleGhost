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
//	CGO_ENABLED=1 gomobile bind -target=android -androidapi 21 -o teleghost.aar ./mobile
package mobile

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"teleghost/internal/appcore"
)

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

// MobilePlatform — no-op реализация PlatformServices для Android.
// Файловые диалоги и буфер обмена недоступны через Go на мобилке.
type MobilePlatform struct{}

func (p *MobilePlatform) OpenFileDialog(title string, filters []string) (string, error) {
	return "", fmt.Errorf("file dialogs not available on mobile")
}
func (p *MobilePlatform) SaveFileDialog(title, defaultFilename string) (string, error) {
	return "", fmt.Errorf("file dialogs not available on mobile")
}
func (p *MobilePlatform) ClipboardSet(text string) {}
func (p *MobilePlatform) ClipboardGet() (string, error) {
	return "", fmt.Errorf("clipboard not available on mobile")
}
func (p *MobilePlatform) ShowWindow() {}
func (p *MobilePlatform) HideWindow() {}

func (p *MobilePlatform) Notify(title, message string) {
	log.Printf("[Mobile] Notification: %s - %s", title, message)
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

	// HTTP сервер
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", handleAPI)
	mux.HandleFunc("/api/events", handleSSE)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

	result, err := dispatch(globalApp, method, req.Args)
	if err != nil {
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

	// === Folders ===
	case "CreateFolder":
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
	case "CopyToClipboard":
		var text string
		parseArgs(args, &text)
		app.CopyToClipboard(text)
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

	case "SelectFiles", "SelectImage":
		return nil, fmt.Errorf("file selection not available on mobile via HTTP bridge")

	case "OpenFile", "ShowInFolder":
		return nil, fmt.Errorf("system file opening not available on mobile")

	case "SaveFileToLocation":
		return nil, fmt.Errorf("file saving to location not available on mobile")

	case "GetImageThumbnail":
		// На мобилке обычно фронтенд сам может это делать или через другой URL
		return "", nil

	case "SaveRouterSettings":
		var settings map[string]interface{}
		parseArgs(args, &settings)
		return nil, app.SaveRouterSettings(settings)

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
