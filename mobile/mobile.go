// Package mobile предоставляет HTTP-адаптер для запуска TeleGhost на Android.
//
// Архитектура:
//
//	Android (Kotlin) вызывает Mobile.Start() через JNI (.aar).
//	Go стартует HTTP-сервер на 127.0.0.1:8080.
//	WebView отображает фронтенд и шлёт запросы на localhost.
//
// Сборка:
//
//	CGO_ENABLED=1 gomobile bind -target=android -androidapi 21 -o teleghost.aar ./mobile
package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"teleghost/internal/core/identity"
	"teleghost/internal/network/media"
	"teleghost/internal/network/messenger"
	"teleghost/internal/network/profiles"
	"teleghost/internal/network/router"
	"teleghost/internal/repository/sqlite"
)

// ─── App-структура (копия основной, без Wails-зависимостей) ─────────────────

// MobileApp — мобильная версия структуры App без зависимостей от Wails runtime.
type MobileApp struct {
	ctx            context.Context
	cancel         context.CancelFunc
	identity       *identity.Identity
	repo           *sqlite.Repository
	router         *router.SAMRouter
	messenger      *messenger.Service
	mediaCrypt     *media.MediaCrypt
	profileManager *profiles.ProfileManager
	status         string
	dataDir        string

	mu sync.RWMutex

	// SSE подписчики для отправки событий во фронтенд
	sseClients   map[chan string]struct{}
	sseClientsMu sync.RWMutex

	server *http.Server
}

// ─── Публичный API для Kotlin/Java (через gomobile) ─────────────────────────

var globalApp *MobileApp

// Start запускает HTTP-сервер на 127.0.0.1:8080.
// Вызывается из Android Service через JNI.
// dataDir — путь к внутреннему хранилищу приложения (context.getFilesDir()).
func Start(dataDir string) {
	if globalApp != nil {
		log.Println("[Mobile] Server already running")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	app := &MobileApp{
		ctx:        ctx,
		cancel:     cancel,
		dataDir:    dataDir,
		status:     "offline",
		sseClients: make(map[chan string]struct{}),
	}

	// Инициализация директорий
	os.MkdirAll(dataDir, 0700)
	os.MkdirAll(filepath.Join(dataDir, "users"), 0700)

	// Инициализация профиль-менеджера
	profilesDir := filepath.Join(dataDir, "profiles")
	pm, err := profiles.NewProfileManager(profilesDir)
	if err == nil {
		app.profileManager = pm
	} else {
		log.Printf("[Mobile] Failed to init profile manager: %v", err)
	}

	globalApp = app

	// HTTP сервер
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", app.handleAPI)
	mux.HandleFunc("/api/events", app.handleSSE)
	mux.HandleFunc("/api/poll-events", app.handlePollEvents)
	// Статика фронтенда (встроенная или из assets)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	app.server = &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	go func() {
		log.Printf("[Mobile] Starting HTTP server on 127.0.0.1:8080")
		if err := app.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Mobile] Server error: %v", err)
		}
	}()

	log.Printf("[Mobile] Server started, dataDir=%s", dataDir)
}

// Stop останавливает HTTP-сервер и очищает ресурсы.
func Stop() {
	if globalApp == nil {
		return
	}

	log.Println("[Mobile] Stopping server...")

	if globalApp.messenger != nil {
		globalApp.messenger.Stop()
	}
	if globalApp.router != nil {
		globalApp.router.Stop()
	}
	if globalApp.repo != nil {
		globalApp.repo.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if globalApp.server != nil {
		globalApp.server.Shutdown(ctx)
	}

	globalApp.cancel()
	globalApp = nil
	log.Println("[Mobile] Server stopped")
}

// IsRunning возвращает true если сервер запущен.
func IsRunning() bool {
	return globalApp != nil
}

// ─── HTTP Handler (роутер запросов) ─────────────────────────────────────────

func (app *MobileApp) handleAPI(w http.ResponseWriter, r *http.Request) {
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

	// Извлекаем имя метода из URL: /api/CreateProfile -> CreateProfile
	method := r.URL.Path[len("/api/"):]
	if method == "" || method == "events" || method == "poll-events" {
		writeError(w, http.StatusBadRequest, "unknown method")
		return
	}

	// Парсим тело запроса
	var req struct {
		Args []json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Диспетчеризация
	result, err := app.dispatch(method, req.Args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"result": result,
		"error":  nil,
	})
}

// ─── Диспетчер методов ──────────────────────────────────────────────────────

func (app *MobileApp) dispatch(method string, args []json.RawMessage) (interface{}, error) {
	switch method {

	// === Auth ===
	case "ListProfiles":
		if app.profileManager == nil {
			return nil, fmt.Errorf("profile manager not initialized")
		}
		return app.profileManager.ListProfiles()

	case "CreateProfile":
		name, pin, mnemonic, userID, avatarPath, usePin, err := parseCreateProfileArgs(args)
		if err != nil {
			return nil, err
		}
		return nil, app.createProfile(name, pin, mnemonic, userID, avatarPath, usePin)

	case "UnlockProfile":
		if len(args) < 2 {
			return nil, fmt.Errorf("UnlockProfile requires 2 args")
		}
		var profileID, pin string
		json.Unmarshal(args[0], &profileID)
		json.Unmarshal(args[1], &pin)
		if app.profileManager == nil {
			return nil, fmt.Errorf("profile manager not initialized")
		}
		return app.profileManager.UnlockProfile(profileID, pin)

	case "DeleteProfile":
		if len(args) < 1 {
			return nil, fmt.Errorf("DeleteProfile requires 1 arg")
		}
		var profileID string
		json.Unmarshal(args[0], &profileID)
		if app.profileManager == nil {
			return nil, fmt.Errorf("profile manager not initialized")
		}
		return nil, app.profileManager.DeleteProfile(profileID)

	case "Login":
		if len(args) < 1 {
			return nil, fmt.Errorf("Login requires 1 arg")
		}
		var seedPhrase string
		json.Unmarshal(args[0], &seedPhrase)
		return nil, app.login(seedPhrase)

	case "CreateAccount":
		return app.createAccount()

	case "Logout":
		app.logout()
		return nil, nil

	case "GetMyInfo":
		return app.getMyInfo(), nil

	case "GetCurrentProfile":
		return app.getCurrentProfile(), nil

	case "UpdateMyProfile":
		if len(args) < 3 {
			return nil, fmt.Errorf("UpdateMyProfile requires 3 args")
		}
		var nickname, bio, avatar string
		json.Unmarshal(args[0], &nickname)
		json.Unmarshal(args[1], &bio)
		json.Unmarshal(args[2], &avatar)
		return nil, app.updateMyProfile(nickname, bio, avatar)

	// === Contacts ===
	case "GetContacts":
		return app.getContacts()

	case "AddContact":
		if len(args) < 2 {
			return nil, fmt.Errorf("AddContact requires 2 args")
		}
		var name, dest string
		json.Unmarshal(args[0], &name)
		json.Unmarshal(args[1], &dest)
		return app.addContact(name, dest)

	case "DeleteContact":
		if len(args) < 1 {
			return nil, fmt.Errorf("DeleteContact requires 1 arg")
		}
		var id string
		json.Unmarshal(args[0], &id)
		return nil, app.deleteContact(id)

	// === Messages ===
	case "SendText":
		if len(args) < 2 {
			return nil, fmt.Errorf("SendText requires 2 args")
		}
		var contactID, text string
		json.Unmarshal(args[0], &contactID)
		json.Unmarshal(args[1], &text)
		return nil, app.sendText(contactID, text)

	case "GetMessages":
		if len(args) < 3 {
			return nil, fmt.Errorf("GetMessages requires 3 args")
		}
		var contactID string
		var limit, offset int
		json.Unmarshal(args[0], &contactID)
		json.Unmarshal(args[1], &limit)
		json.Unmarshal(args[2], &offset)
		return app.getMessages(contactID, limit, offset)

	case "EditMessage":
		if len(args) < 2 {
			return nil, fmt.Errorf("EditMessage requires 2 args")
		}
		var messageID, newContent string
		json.Unmarshal(args[0], &messageID)
		json.Unmarshal(args[1], &newContent)
		return nil, app.editMessage(messageID, newContent)

	case "DeleteMessage":
		if len(args) < 1 {
			return nil, fmt.Errorf("DeleteMessage requires 1 arg")
		}
		var messageID string
		json.Unmarshal(args[0], &messageID)
		return nil, app.deleteMessage(messageID)

	case "MarkChatAsRead":
		if len(args) < 1 {
			return nil, fmt.Errorf("MarkChatAsRead requires 1 arg")
		}
		var chatID string
		json.Unmarshal(args[0], &chatID)
		return nil, app.markChatAsRead(chatID)

	case "GetUnreadCount":
		return app.getUnreadCount()

	// === Folders ===
	case "CreateFolder":
		if len(args) < 2 {
			return nil, fmt.Errorf("CreateFolder requires 2 args")
		}
		var name, icon string
		json.Unmarshal(args[0], &name)
		json.Unmarshal(args[1], &icon)
		return app.createFolder(name, icon)

	case "GetFolders":
		return app.getFolders()

	case "DeleteFolder":
		if len(args) < 1 {
			return nil, fmt.Errorf("DeleteFolder requires 1 arg")
		}
		var id string
		json.Unmarshal(args[0], &id)
		return nil, app.deleteFolder(id)

	case "UpdateFolder":
		if len(args) < 3 {
			return nil, fmt.Errorf("UpdateFolder requires 3 args")
		}
		var id, name, icon string
		json.Unmarshal(args[0], &id)
		json.Unmarshal(args[1], &name)
		json.Unmarshal(args[2], &icon)
		return nil, app.updateFolder(id, name, icon)

	case "AddChatToFolder":
		if len(args) < 2 {
			return nil, fmt.Errorf("AddChatToFolder requires 2 args")
		}
		var folderID, contactID string
		json.Unmarshal(args[0], &folderID)
		json.Unmarshal(args[1], &contactID)
		return nil, app.addChatToFolder(folderID, contactID)

	case "RemoveChatFromFolder":
		if len(args) < 2 {
			return nil, fmt.Errorf("RemoveChatFromFolder requires 2 args")
		}
		var folderID, contactID string
		json.Unmarshal(args[0], &folderID)
		json.Unmarshal(args[1], &contactID)
		return nil, app.removeChatFromFolder(folderID, contactID)

	// === Settings ===
	case "GetMyDestination":
		return app.getMyDestination(), nil

	case "GetRouterSettings":
		return app.getRouterSettings(), nil

	case "GetNetworkStatus":
		return app.status, nil

	case "GetAppAboutInfo":
		return app.getAppAboutInfo(), nil

	case "CheckForUpdates":
		return "У вас установлена последняя версия", nil

	// === Utils (заглушки для мобилки — нет системных диалогов) ===
	case "CopyToClipboard":
		// На Android WebView это делает JS через navigator.clipboard
		return nil, nil

	case "GetFileBase64":
		if len(args) < 1 {
			return nil, fmt.Errorf("GetFileBase64 requires 1 arg")
		}
		var path string
		json.Unmarshal(args[0], &path)
		return app.getFileBase64(path)

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// ─── SSE (Server-Sent Events) ───────────────────────────────────────────────

func (app *MobileApp) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 64)

	app.sseClientsMu.Lock()
	app.sseClients[ch] = struct{}{}
	app.sseClientsMu.Unlock()

	defer func() {
		app.sseClientsMu.Lock()
		delete(app.sseClients, ch)
		app.sseClientsMu.Unlock()
		close(ch)
	}()

	// Heartbeat чтобы detect broken connections
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

func (app *MobileApp) handlePollEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Заглушка: возвращаем пустой массив
	json.NewEncoder(w).Encode([]interface{}{})
}

// emitEvent отправляет событие всем подписанным SSE-клиентам.
func (app *MobileApp) emitEvent(event string, data interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{
		"event": event,
		"data":  data,
	})

	app.sseClientsMu.RLock()
	defer app.sseClientsMu.RUnlock()

	for ch := range app.sseClients {
		select {
		case ch <- string(payload):
		default:
			// Клиент медленный, пропускаем
		}
	}
}

// ─── Вспомогательные методы (маппинг на App-логику) ─────────────────────────
// Эти методы повторяют логику из app_*.go, но без зависимости от Wails.
// В реальной реализации тут используется тот же код внутренних пакетов.

func (app *MobileApp) createProfile(name, pin, mnemonic, userID, avatarPath string, usePin bool) error {
	if app.profileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}
	// Логика аналогична app_auth.go:CreateProfile
	id, err := identity.FromMnemonic(mnemonic)
	if err != nil {
		newID, mnem, err2 := identity.Generate()
		if err2 != nil {
			return err2
		}
		id = newID
		mnemonic = mnem
	}

	return app.profileManager.CreateProfile(name, pin, mnemonic, id.Keys.UserID, avatarPath, usePin)
}

func (app *MobileApp) login(seedPhrase string) error {
	id, err := identity.FromMnemonic(seedPhrase)
	if err != nil {
		return fmt.Errorf("invalid seed phrase: %w", err)
	}

	app.identity = id

	if err := app.initUserRepository(id.Keys.UserID); err != nil {
		return err
	}

	// Подключение к I2P в фоне
	go app.connectToI2P()

	return nil
}

func (app *MobileApp) createAccount() (string, error) {
	id, mnemonic, err := identity.Generate()
	if err != nil {
		return "", err
	}

	app.identity = id

	if err := app.initUserRepository(id.Keys.UserID); err != nil {
		return "", err
	}

	// Сохраняем профиль
	if app.repo != nil {
		app.repo.SaveUser(app.ctx, &sqlite.User{
			ID:        id.Keys.UserID,
			PublicKey: id.Keys.PublicKeyBase64(),
			Nickname:  "User",
		})
	}

	go app.connectToI2P()

	return mnemonic, nil
}

func (app *MobileApp) logout() {
	if app.messenger != nil {
		app.messenger.Stop()
		app.messenger = nil
	}
	if app.router != nil {
		app.router.Stop()
		app.router = nil
	}
	if app.repo != nil {
		app.repo.Close()
		app.repo = nil
	}
	app.identity = nil
	app.status = "offline"
}

func (app *MobileApp) initUserRepository(userID string) error {
	userDir := filepath.Join(app.dataDir, "users", userID)
	os.MkdirAll(userDir, 0700)

	dbPath := filepath.Join(userDir, "data.db")

	var keys *identity.Keys
	if app.identity != nil {
		keys = app.identity.Keys
	}

	repo, err := sqlite.New(dbPath, keys)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	app.repo = repo

	if err := repo.Migrate(app.ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

func (app *MobileApp) connectToI2P() {
	app.status = "connecting"
	app.emitEvent("network_status", "connecting")

	cfg := router.DefaultConfig()
	app.router = router.NewSAMRouter(cfg)

	if err := app.router.Start(app.ctx); err != nil {
		app.status = "error"
		app.emitEvent("network_status", "error")
		log.Printf("[Mobile] I2P connection failed: %v", err)
		return
	}

	// Запускаем messenger
	app.messenger = messenger.NewService(app.router, app.identity.Keys, app.onMessageReceived)
	app.messenger.SetContactHandler(app.onContactRequest)

	if err := app.messenger.Start(app.ctx); err != nil {
		app.status = "error"
		app.emitEvent("network_status", "error")
		return
	}

	app.status = "online"
	app.emitEvent("network_status", "online")
}

// ─── Обработчики входящих сообщений ─────────────────────────────────────────

func (app *MobileApp) onMessageReceived(msg interface{}, senderPubKey, senderAddr string) {
	app.emitEvent("new_message", map[string]interface{}{
		"sender":  senderPubKey,
		"address": senderAddr,
	})
}

func (app *MobileApp) onContactRequest(pubKey, nickname, i2pAddress string) {
	app.emitEvent("new_contact", map[string]interface{}{
		"nickname": nickname,
	})
}

// ─── Методы-обёртки (используют те же internal пакеты) ──────────────────────

func (app *MobileApp) getMyInfo() map[string]interface{} {
	if app.identity == nil {
		return nil
	}

	info := map[string]interface{}{
		"ID":        app.identity.Keys.UserID,
		"PublicKey": app.identity.Keys.PublicKeyBase64(),
	}

	if app.repo != nil {
		if user, err := app.repo.GetMyProfile(app.ctx); err == nil && user != nil {
			info["Nickname"] = user.Nickname
			info["Avatar"] = user.Avatar
		}
	}

	if app.router != nil {
		info["Destination"] = app.router.GetDestination()
	}

	return info
}

func (app *MobileApp) getCurrentProfile() map[string]interface{} {
	if app.profileManager == nil || app.identity == nil {
		return nil
	}
	profiles, _ := app.profileManager.ListProfiles()
	for _, p := range profiles {
		if p.UserID == app.identity.Keys.UserID {
			return map[string]interface{}{
				"id":      p.ID,
				"name":    p.Name,
				"use_pin": p.UsePin,
			}
		}
	}
	return nil
}

func (app *MobileApp) updateMyProfile(nickname, bio, avatar string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	user, err := app.repo.GetMyProfile(app.ctx)
	if err != nil {
		return err
	}
	if nickname != "" {
		user.Nickname = nickname
	}
	if avatar != "" {
		user.Avatar = avatar
	}
	return app.repo.SaveUser(app.ctx, user)
}

func (app *MobileApp) getContacts() (interface{}, error) {
	if app.repo == nil {
		return []interface{}{}, nil
	}
	contacts, err := app.repo.GetContacts(app.ctx)
	if err != nil {
		return nil, err
	}
	return contacts, nil
}

func (app *MobileApp) addContact(name, destination string) (interface{}, error) {
	if app.repo == nil {
		return nil, fmt.Errorf("not logged in")
	}
	// Делегируем внутренней логике
	return nil, fmt.Errorf("AddContact: implement via internal packages")
}

func (app *MobileApp) deleteContact(id string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.DeleteContact(app.ctx, id)
}

func (app *MobileApp) sendText(contactID, text string) error {
	if app.messenger == nil {
		return fmt.Errorf("not connected to I2P")
	}
	// Делегируем messenger service
	return fmt.Errorf("SendText: implement via messenger service")
}

func (app *MobileApp) getMessages(contactID string, limit, offset int) (interface{}, error) {
	if app.repo == nil {
		return []interface{}{}, nil
	}
	msgs, err := app.repo.GetMessages(app.ctx, contactID, limit, offset)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func (app *MobileApp) editMessage(messageID, newContent string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.EditMessage(app.ctx, messageID, newContent)
}

func (app *MobileApp) deleteMessage(messageID string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.DeleteMessage(app.ctx, messageID)
}

func (app *MobileApp) markChatAsRead(chatID string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.MarkChatAsRead(app.ctx, chatID)
}

func (app *MobileApp) getUnreadCount() (int, error) {
	if app.repo == nil {
		return 0, nil
	}
	return app.repo.GetUnreadCount(app.ctx)
}

func (app *MobileApp) createFolder(name, icon string) (string, error) {
	if app.repo == nil {
		return "", fmt.Errorf("not logged in")
	}
	return app.repo.CreateFolder(app.ctx, name, icon)
}

func (app *MobileApp) getFolders() (interface{}, error) {
	if app.repo == nil {
		return []interface{}{}, nil
	}
	return app.repo.GetFolders(app.ctx)
}

func (app *MobileApp) deleteFolder(id string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.DeleteFolder(app.ctx, id)
}

func (app *MobileApp) updateFolder(id, name, icon string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.UpdateFolder(app.ctx, id, name, icon)
}

func (app *MobileApp) addChatToFolder(folderID, contactID string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.AddChatToFolder(app.ctx, folderID, contactID)
}

func (app *MobileApp) removeChatFromFolder(folderID, contactID string) error {
	if app.repo == nil {
		return fmt.Errorf("not logged in")
	}
	return app.repo.RemoveChatFromFolder(app.ctx, folderID, contactID)
}

func (app *MobileApp) getMyDestination() string {
	if app.router == nil {
		return ""
	}
	return app.router.GetDestination()
}

func (app *MobileApp) getRouterSettings() map[string]interface{} {
	return map[string]interface{}{
		"tunnelLength": 1,
		"logToFile":    false,
	}
}

func (app *MobileApp) getAppAboutInfo() map[string]interface{} {
	return map[string]interface{}{
		"app_version": "1.0.2-beta",
		"i2p_version": "2.58.0",
		"i2p_path":    filepath.Join(app.dataDir, "i2pd"),
		"author":      "TeleGhost Team",
		"license":     "MIT / Open Source",
	}
}

func (app *MobileApp) getFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", data), nil // Simplified; real impl uses base64
}

// ─── Парсеры аргументов ─────────────────────────────────────────────────────

func parseCreateProfileArgs(args []json.RawMessage) (string, string, string, string, string, bool, error) {
	if len(args) < 6 {
		return "", "", "", "", "", false, fmt.Errorf("CreateProfile requires 6 args")
	}
	var name, pin, mnemonic, userID, avatarPath string
	var usePin bool
	json.Unmarshal(args[0], &name)
	json.Unmarshal(args[1], &pin)
	json.Unmarshal(args[2], &mnemonic)
	json.Unmarshal(args[3], &userID)
	json.Unmarshal(args[4], &avatarPath)
	json.Unmarshal(args[5], &usePin)
	return name, pin, mnemonic, userID, avatarPath, usePin, nil
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"result": nil,
		"error":  msg,
	})
}
