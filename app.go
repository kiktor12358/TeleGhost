package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"teleghost/internal/appcore"
	"teleghost/internal/network/media"

	"github.com/gen2brain/beeep"
	"github.com/nfnt/resize"
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

// FolderInfo информация о папке
type FolderInfo struct {
	ID          string
	Name        string
	Icon        string
	ChatIDs     []string
	Position    int
	UnreadCount int
}

// ContactInfo информация о контакте для фронтенда
type ContactInfo struct {
	ID              string
	Nickname        string
	PublicKey       string
	Avatar          string
	I2PAddress      string
	LastMessage     string
	LastMessageTime int64
	LastSeen        string
	IsOnline        bool
	ChatID          string
	UnreadCount     int
}

// MessageInfo сообщение для фронтенда
type MessageInfo struct {
	ID          string
	Content     string
	Timestamp   int64
	IsOutgoing  bool
	Status      string
	ContentType string
	FileCount   int
	TotalSize   int64
	Filenames   []string
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
	Mnemonic    string
}

// AppAboutInfo информация о приложении
type AppAboutInfo struct {
	AppVersion string `json:"app_version"`
	I2PVersion string `json:"i2p_version"`
	I2PPath    string `json:"i2p_path"`
	Author     string `json:"author"`
	License    string `json:"license"`
}

// RouterSettings настройки роутера
type RouterSettings struct {
	TunnelLength int // 1, 3, 5
	LogToFile    bool
}

// App основная структура приложения
type App struct {
	ctx  context.Context
	core *appcore.AppCore

	mediaCrypt     *media.MediaCrypt
	embeddedRouter interface {
		IsReady() bool
		Start(context.Context) error
		Stop() error
	}
	embeddedStop func() error
	trayManager  *TrayManager
}

// WailsEmitter — реализация appcore.EventEmitter через Wails runtime
type WailsEmitter struct {
	ctx context.Context
}

func (e *WailsEmitter) Emit(event string, data ...interface{}) {
	runtime.EventsEmit(e.ctx, event, data...)
}

// WailsPlatform — реализация appcore.PlatformServices через Wails runtime
type WailsPlatform struct {
	ctx context.Context
}

func (p *WailsPlatform) OpenFileDialog(title string, filters []string) (string, error) {
	return runtime.OpenFileDialog(p.ctx, runtime.OpenDialogOptions{
		Title: title,
	})
}

func (p *WailsPlatform) SaveFileDialog(title, defaultFilename string) (string, error) {
	return runtime.SaveFileDialog(p.ctx, runtime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultFilename,
	})
}

func (p *WailsPlatform) ClipboardSet(text string) {
	runtime.ClipboardSetText(p.ctx, text)
}

func (p *WailsPlatform) ClipboardGet() (string, error) {
	return runtime.ClipboardGetText(p.ctx)
}

func (p *WailsPlatform) ShowWindow() {
	runtime.WindowShow(p.ctx)
}

func (p *WailsPlatform) HideWindow() {
	runtime.WindowHide(p.ctx)
}

func (p *WailsPlatform) Notify(title, message string) {
	// Десктопная реализация Notify через beeep
	appIcon := "" // Можно добавить путь к иконке
	if err := beeep.Notify(title, message, appIcon); err != nil {
		log.Printf("[App] Failed to send notification: %v", err)
	}
}

// NewApp creates a new App application struct
func NewApp(iconData []byte) *App {
	app := &App{}
	app.trayManager = NewTrayManager(app, iconData)
	return app
}

// startup вызывается при старте приложения
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := clipboard.Init(); err != nil {
		log.Printf("[App] Failed to init clipboard: %v", err)
	}

	appDataDir, _ := os.UserConfigDir()
	dataDir := filepath.Join(appDataDir, "TeleGhost")

	emitter := &WailsEmitter{ctx: ctx}
	platform := &WailsPlatform{ctx: ctx}

	a.core = appcore.NewAppCore(dataDir, emitter, platform)
	if err := a.core.Init(); err != nil {
		log.Printf("[App] Failed to init app core: %v", err)
	}

	if err := a.initEmbeddedRouter(ctx); err != nil {
		log.Printf("[App] Failed to init embedded router: %v", err)
	}

	if a.trayManager != nil {
		a.trayManager.Start()
	}

	log.Printf("[App] Started. Data dir: %s", dataDir)
}

// shutdown вызывается при закрытии приложения
func (a *App) shutdown(ctx context.Context) {
	log.Printf("[App] Shutting down...")
	if a.core != nil {
		a.core.Shutdown()
	}
	if a.embeddedStop != nil {
		a.embeddedStop()
	}
	if a.trayManager != nil {
		a.trayManager.Stop()
	}
	log.Println("[App] Shutdown complete.")
}

// GetMediaHandler возвращает обработчик для медиафайлов
func (a *App) GetMediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.core == nil || a.core.Identity == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// MediaCrypt инициализируется в AppCore при логине
		mediaDir := filepath.Join(a.core.DataDir, "users", a.core.Identity.Keys.UserID, "media")
		mc, _ := media.NewMediaCrypt(a.core.Identity.Keys.EncryptionKey)
		handler := mc.NewMediaHandler(mediaDir)
		handler.ServeHTTP(w, r)
	})
}

// ShowWindow показывает окно из трея
func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
}

// QuitApp закрывает приложение
func (a *App) QuitApp() {
	runtime.Quit(a.ctx)
}

// SelectImage открывает диалог выбора изображения
func (a *App) SelectImage() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите изображение",
		Filters: []runtime.FileFilter{
			{DisplayName: "Изображения (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
}

// SelectFiles открывает диалог выбора файлов
func (a *App) SelectFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите файлы",
	})
}

// GetImageThumbnail возвращает уменьшенную копию изображения в base64
func (a *App) GetImageThumbnail(path string) (string, error) {
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

// SaveTempImage сохраняет изображение во временную папку для превью
func (a *App) SaveTempImage(base64Data, filename string) (string, error) {
	// Убираем префикс если есть
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}

	tempDir := filepath.Join(a.core.DataDir, "temp")
	os.MkdirAll(tempDir, 0755)

	filePath := filepath.Join(tempDir, filename)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", err
	}

	return filePath, nil
}

// OpenFile открывает файл системным приложением
func (a *App) OpenFile(path string) error {
	runtime.BrowserOpenURL(a.ctx, "file://"+path)
	return nil
}

// ShowInFolder открывает папку с файлом
func (a *App) ShowInFolder(path string) error {
	// Wails v2 не имеет кроссплатформенного ShowInFolder, используем костыль или заглушку
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.Dir(path))
	return nil
}

// SaveFileToLocation сохраняет файл в выбранное место
func (a *App) SaveFileToLocation(path, filename string) (string, error) {
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Сохранить файл",
		DefaultFilename: filename,
	})
	if err != nil || dest == "" {
		return "", err
	}

	// Копируем файл
	input, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(dest, input, 0644); err != nil {
		return "", err
	}

	return dest, nil
}

// RequestProfileUpdate запрашивает обновление профиля (заглушка для совместимости)
func (a *App) RequestProfileUpdate() {
	runtime.EventsEmit(a.ctx, "profile_updated")
}
