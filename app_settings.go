package main

// GetMyDestination возвращает I2P адрес.
func (a *App) GetMyDestination() string {
	return a.core.GetMyDestination()
}

// GetRouterSettings возвращает настройки роутера.
func (a *App) GetRouterSettings() *RouterSettings {
	coreSettings := a.core.GetRouterSettings()
	return &RouterSettings{
		TunnelLength: coreSettings.TunnelLength,
		LogToFile:    coreSettings.LogToFile,
	}
}

// GetAppAboutInfo возвращает информацию о приложении.
func (a *App) GetAppAboutInfo() *AppAboutInfo {
	coreInfo := a.core.GetAppAboutInfo()
	return &AppAboutInfo{
		AppVersion: coreInfo.AppVersion,
		I2PVersion: coreInfo.I2PVersion,
		I2PPath:    coreInfo.I2PPath,
		Author:     coreInfo.Author,
		License:    coreInfo.License,
	}
}

// GetNetworkStatus возвращает статус сети.
func (a *App) GetNetworkStatus() string {
	return a.core.GetNetworkStatus()
}

// SaveRouterSettings сохраняет настройки роутера.
func (a *App) SaveRouterSettings(settings map[string]interface{}) error {
	return a.core.SaveRouterSettings(settings)
}

// CheckForUpdates (заглушка)
func (a *App) CheckForUpdates() string {
	return "У вас установлена последняя версия"
}
