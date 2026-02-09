package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"teleghost/internal/core/identity"
	"teleghost/internal/network/media"
	"teleghost/internal/network/messenger"
	"teleghost/internal/network/profiles"
	"teleghost/internal/network/router"
	"teleghost/internal/repository/sqlite"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/clipboard"
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
	ID          string
	Nickname    string
	PublicKey   string
	Avatar      string
	I2PAddress  string
	LastMessage string
	LastSeen    string
	IsOnline    bool
	ChatID      string
}

// MessageInfo сообщение для фронтенда
type MessageInfo struct {
	ID          string
	Content     string
	Timestamp   int64
	IsOutgoing  bool
	Status      string
	Attachments []map[string]interface{}
}

// UserInfo информация о текущем пользователе
type UserInfo struct {
	ID          string
	Nickname    string
	Avatar      string
	PublicKey   string
	Destination string
	Fingerprint string
}

// App основная структура приложения
type App struct {
	ctx            context.Context
	identity       *identity.Identity
	repo           *sqlite.Repository
	router         *router.SAMRouter
	messenger      *messenger.Service
	mediaCrypt     *media.MediaCrypt
	profileManager *profiles.ProfileManager
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

	routerSettings *RouterSettings
}

// RouterSettings настройки роутера
type RouterSettings struct {
	TunnelLength int // 1, 3, 5
	LogToFile    bool
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

	if err := clipboard.Init(); err != nil {
		log.Printf("[App] Failed to init clipboard: %v", err)
	}

	os.MkdirAll(a.dataDir, 0700)
	os.MkdirAll(filepath.Join(a.dataDir, "users"), 0700)

	a.loadRouterSettings()

	if err := a.initEmbeddedRouter(ctx); err != nil {
		log.Printf("[App] Failed to init embedded router: %v", err)
	}

	if a.trayManager != nil {
		a.trayManager.Start()
	}

	profilesDir := filepath.Join(a.dataDir, "profiles")
	pm, err := profiles.NewProfileManager(profilesDir)
	if err == nil {
		a.profileManager = pm
	} else {
		log.Printf("[App] Failed to init profile manager: %v", err)
	}

	log.Printf("[App] Started. Data dir: %s", a.dataDir)
}

// shutdown вызывается при закрытии приложения
func (a *App) shutdown(ctx context.Context) {
	log.Printf("[App] Shutting down...")

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
		a.embeddedStop()
	}

	if a.trayManager != nil {
		a.trayManager.Stop()
	}

	log.Println("[App] Shutdown complete.")
}

// initUserRepository инициализирует репозиторий для конкретного пользователя
func (a *App) initUserRepository(userID string) error {
	userDir := filepath.Join(a.dataDir, "users", userID)
	if err := os.MkdirAll(userDir, 0700); err != nil {
		return fmt.Errorf("failed to create user directory: %w", err)
	}

	dbPath := filepath.Join(userDir, "data.db")

	var keys *identity.Keys
	if a.identity != nil {
		keys = a.identity.Keys
	}

	repo, err := sqlite.New(dbPath, keys)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	a.repo = repo

	if err := repo.Migrate(a.ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if err := repo.MigrateEncryption(a.ctx); err != nil {
		log.Printf("[App] Encryption migration failed: %v", err)
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

// GetMediaHandler возвращает обработчик для зашифрованных медиафайлов
func (a *App) GetMediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.mediaCrypt == nil || a.identity == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		mediaDir := filepath.Join(a.dataDir, "users", a.identity.Keys.UserID, "media")
		handler := a.mediaCrypt.NewMediaHandler(mediaDir)
		handler.ServeHTTP(w, r)
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
