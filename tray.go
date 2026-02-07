package main

import (
	"log"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// TrayManager управляет иконкой в системном трее
type TrayManager struct {
	app       *App
	iconData  []byte
	isVisible bool
}

// NewTrayManager создаёт менеджер трея
func NewTrayManager(app *App, iconData []byte) *TrayManager {
	return &TrayManager{app: app, iconData: iconData, isVisible: true}
}

// Start запускает system tray в отдельной горутине
func (t *TrayManager) Start() {
	go func() {
		systray.Run(t.onReady, t.onExit)
	}()
}

// onReady вызывается когда трей готов
func (t *TrayManager) onReady() {
	systray.SetIcon(t.iconData)
	systray.SetTitle("TeleGhost")
	systray.SetTooltip("TeleGhost — Анонимный мессенджер I2P")

	// Обрабоботка клика по иконке (ЛКМ)
	// Используем SetOnTapped для fyne.io/systray v1.12.0+
	systray.SetOnTapped(func() {
		t.toggleWindow()
	})

	// Пункты меню (ПКМ)
	mShow := systray.AddMenuItem("Показать/Скрыть", "Переключить видимость окна")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Выход", "Закрыть приложение полностью")

	// Обработка кликов меню
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				t.toggleWindow()
			case <-mQuit.ClickedCh:
				log.Println("[Tray] Quit requested")
				systray.Quit()
				if t.app != nil && t.app.ctx != nil {
					runtime.Quit(t.app.ctx)
				}
			}
		}
	}()
}

// toggleWindow переключает видимость окна
func (t *TrayManager) toggleWindow() {
	if t.app == nil || t.app.ctx == nil {
		return
	}

	if t.isVisible {
		runtime.WindowHide(t.app.ctx)
		t.isVisible = false
	} else {
		runtime.WindowShow(t.app.ctx)
		// runtime.WindowSetFocus не найден, WindowShow должен поднять окно
		t.isVisible = true
	}
}

// Stop останавливает трей
func (t *TrayManager) Stop() {
	systray.Quit()
}

// onExit вызывается при выходе
func (t *TrayManager) onExit() {
	log.Println("[Tray] Exiting...")
}
