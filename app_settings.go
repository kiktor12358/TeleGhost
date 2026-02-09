package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"teleghost/internal/network/messenger"
	"teleghost/internal/network/router"

	"github.com/go-i2p/i2pkeys"
)

// getRouterSettingsPath возвращает путь к файлу настроек роутера
func (a *App) getRouterSettingsPath() string {
	return filepath.Join(a.dataDir, "router_settings.json")
}

// loadRouterSettings загружает настройки роутера
func (a *App) loadRouterSettings() {
	path := a.getRouterSettingsPath()
	DEFAULT_SETTINGS := &RouterSettings{
		TunnelLength: 1,
		LogToFile:    false,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		a.routerSettings = DEFAULT_SETTINGS
		return
	}

	var settings RouterSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		a.routerSettings = DEFAULT_SETTINGS
		return
	}
	a.routerSettings = &settings
}

// saveRouterSettings сохраняет настройки роутера
func (a *App) saveRouterSettings() error {
	path := a.getRouterSettingsPath()
	data, err := json.MarshalIndent(a.routerSettings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetRouterSettings возвращает текущие настройки роутера
func (a *App) GetRouterSettings() *RouterSettings {
	if a.routerSettings == nil {
		a.loadRouterSettings()
	}
	return a.routerSettings
}

// SaveRouterSettings сохраняет новые настройки
func (a *App) SaveRouterSettings(settings RouterSettings) error {
	a.routerSettings = &settings
	return a.saveRouterSettings()
}

// connectToI2P подключение к I2P сети
func (a *App) connectToI2P() {
	a.setNetworkStatus(NetworkStatusConnecting)

	cfg := router.DefaultConfig()
	settings := a.GetRouterSettings()
	if settings != nil {
		cfg.InboundLength = settings.TunnelLength
		cfg.OutboundLength = settings.TunnelLength
	}
	a.router = router.NewSAMRouter(cfg)

	if a.embeddedRouter != nil {
		for i := 0; i < 30; i++ {
			if a.embeddedRouter.IsReady() {
				break
			}
			select {
			case <-a.ctx.Done():
				return
			default:
				log.Println("Waiting for embedded router...")
				// time.Sleep(1 * time.Second) // In external file, maybe use time package
			}
		}
	}

	userDir := filepath.Join(a.dataDir, "users", a.identity.Keys.UserID)
	keysPath := filepath.Join(userDir, "i2p.keys")

	if data, err := os.ReadFile(keysPath); err == nil {
		if a.identity != nil && a.identity.Keys != nil {
			decrypted, err := a.identity.Keys.Decrypt(data)
			if err == nil {
				reader := bytes.NewReader(decrypted)
				keys, err := i2pkeys.LoadKeysIncompat(reader)
				if err == nil {
					a.router.SetKeys(keys)
				}
			}
		}
	}

	if err := a.router.Start(a.ctx); err != nil {
		a.setNetworkStatus(NetworkStatusError)
		return
	}

	currentKeys := a.router.GetKeys()
	var buf bytes.Buffer
	if err := i2pkeys.StoreKeysIncompat(currentKeys, &buf); err == nil {
		data := buf.Bytes()
		if a.identity != nil && a.identity.Keys != nil {
			encrypted, err := a.identity.Keys.Encrypt(data)
			if err == nil {
				data = encrypted
			}
		}
		os.WriteFile(keysPath, data, 0600)

		if a.repo != nil && a.identity != nil {
			existingUser, _ := a.repo.GetMyProfile(a.ctx)
			if existingUser != nil {
				existingUser.I2PAddress = a.router.GetDestination()
				existingUser.I2PKeys = buf.Bytes()
				a.repo.SaveUser(a.ctx, existingUser)
			}
		}
	}

	a.messenger = messenger.NewService(a.router, a.identity.Keys, a.onMessageReceived)
	a.messenger.SetContactHandler(a.onContactRequest)

	nickname := "User"
	if profile, err := a.repo.GetMyProfile(a.ctx); err == nil && profile != nil {
		nickname = profile.Nickname
	}
	a.messenger.SetNickname(nickname)
	a.messenger.SetProfileUpdateHandler(a.onProfileUpdate)
	a.messenger.SetProfileRequestHandler(a.onProfileRequest)
	// a.messenger.SetFileOfferHandler(a.onFileOffer)
	// a.messenger.SetFileResponseHandler(a.onFileResponse)
	// a.messenger.SetAttachmentSaver(a.saveAttachment)

	if err := a.messenger.Start(a.ctx); err != nil {
		a.setNetworkStatus(NetworkStatusError)
		return
	}

	a.setNetworkStatus(NetworkStatusOnline)
}
