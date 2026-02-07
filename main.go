package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed icon.png
var iconPNG []byte

//go:embed icon_tray.png
var iconTrayPNG []byte

func main() {
	// Временное решение для проблем с WebKit/NVIDIA/Wayland на Linux
	// Пользователи сообщают, что без этого флага приложение падает или работает некорректно
	os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")

	// Также можно принудительно попробовать X11, если Wayland совсем не работает,
	// но пока ограничимся композитингом, так как это помогло.
	// os.Setenv("GDK_BACKEND", "x11")

	// Create an instance of the app structure
	app := NewApp(iconTrayPNG)

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "TeleGhost",
		Width:     1200,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 23, G: 33, B: 43, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// Минимизация в трей при закрытии на крестик
		HideWindowOnClose: true,
		Bind: []interface{}{
			app,
		},
		// Linux-специфичные опции (включая иконку в трее)
		Linux: &linux.Options{
			Icon:                iconPNG,
			WindowIsTranslucent: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
