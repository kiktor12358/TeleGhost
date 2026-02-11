package appcore

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// GetMyDestination возвращает I2P адрес.
func (a *AppCore) GetMyDestination() string {
	if a.Messenger == nil {
		return ""
	}
	return a.Messenger.GetDestination()
}

// GetRouterSettings возвращает настройки роутера.
func (a *AppCore) GetRouterSettings() *RouterSettings {
	settingsFile := filepath.Join(a.DataDir, "router_settings.json")

	// Значения по умолчанию
	defaultSettings := &RouterSettings{
		TunnelLength: 1, // Fast mode by default as requested before
		LogToFile:    false,
	}

	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return defaultSettings
	}

	var settings RouterSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("[AppCore] Failed to parse router settings: %v", err)
		return defaultSettings
	}

	return &settings
}

// GetAppAboutInfo возвращает информацию о приложении.
func (a *AppCore) GetAppAboutInfo() *AppAboutInfo {
	return &AppAboutInfo{
		AppVersion: "1.0.2-beta",
		I2PVersion: "2.58.0",
		I2PPath:    filepath.Join(a.DataDir, "i2pd"),
		Author:     "TeleGhost Team",
		License:    "MIT / Open Source",
	}
}

// SaveRouterSettings сохраняет настройки роутера.
func (a *AppCore) SaveRouterSettings(settings map[string]interface{}) error {
	settingsFile := filepath.Join(a.DataDir, "router_settings.json")

	current := a.GetRouterSettings()

	if val, ok := settings["tunnelLength"].(float64); ok {
		current.TunnelLength = int(val)
	}
	if val, ok := settings["logToFile"].(bool); ok {
		current.LogToFile = val
	}

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsFile, data, 0600)
}

// GetNetworkStatus возвращает текущий статус сети
func (a *AppCore) GetNetworkStatus() string {
	return string(a.Status)
}
