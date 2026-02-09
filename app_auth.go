package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
	"teleghost/internal/network/media"
	"teleghost/internal/network/profiles"
	pb "teleghost/internal/proto"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/protobuf/proto"
)

// === Profile Management ===

// CreateProfile создает новый зашифрованный профиль
func (a *App) CreateProfile(name string, pin string, mnemonic string, userID string, avatarPath string, usePin bool) error {
	if a.profileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}
	// For new profiles, existingID is empty
	err := a.profileManager.CreateProfile(name, pin, mnemonic, userID, avatarPath, usePin, "")
	if err != nil {
		return err
	}

	// Fix: Sync profile to DB if this is our current user
	if a.identity != nil && a.identity.Keys.UserID == userID {
		if a.repo != nil {
			// We might want to update the DB profile too
			profile, _ := a.repo.GetMyProfile(a.ctx)
			if profile != nil {
				profile.Nickname = name
				if avatarPath != "" {
					// We need to handle avatar storage properly.
					// For now, let's just update the nickname.
				}
				a.repo.SaveUser(a.ctx, profile)
			}
		}
	}

	return nil
}

// UpdateProfile обновляет профиль
func (a *App) UpdateProfile(profileID string, name string, avatarPath string, deleteAvatar bool, usePin bool, newPin string, mnemonic string) error {
	if a.profileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}
	return a.profileManager.UpdateProfile(profileID, name, avatarPath, deleteAvatar, usePin, newPin, mnemonic)
}

// ListProfiles возвращает список доступных профилей
func (a *App) ListProfiles() ([]profiles.ProfileMetadata, error) {
	if a.profileManager == nil {
		return nil, fmt.Errorf("profile manager not initialized")
	}
	return a.profileManager.ListProfiles()
}

// UnlockProfile проверяет ПИН и возвращает мнемонику
func (a *App) UnlockProfile(profileID string, pin string) (string, error) {
	if a.profileManager == nil {
		return "", fmt.Errorf("profile manager not initialized")
	}
	return a.profileManager.UnlockProfile(profileID, pin)
}

// Login авторизация по seed-фразе
func (a *App) Login(seedPhrase string) error {
	log.Printf("[DEBUG] Login called")
	seedPhrase = strings.TrimSpace(seedPhrase)

	if !identity.ValidateMnemonic(seedPhrase) {
		return fmt.Errorf("invalid seed phrase")
	}

	keys, err := identity.RecoverKeys(seedPhrase)
	if err != nil {
		return fmt.Errorf("failed to recover keys: %w", err)
	}

	a.identity = &identity.Identity{
		Mnemonic: seedPhrase,
		Keys:     keys,
	}

	if err := a.initUserRepository(keys.UserID); err != nil {
		return fmt.Errorf("failed to init user repository: %w", err)
	}

	mc, err := media.NewMediaCrypt(keys.EncryptionKey)
	if err == nil {
		a.mediaCrypt = mc
		mediaDir := filepath.Join(a.dataDir, "users", keys.UserID, "media")
		go mc.MigrateDirectory(mediaDir)
	}

	existingProfile, _ := a.repo.GetMyProfile(a.ctx)
	if existingProfile == nil {
		user := &core.User{
			ID:         keys.UserID,
			PublicKey:  keys.PublicKeyBase64,
			PrivateKey: keys.SigningPrivateKey,
			Mnemonic:   seedPhrase,
			Nickname:   "User",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		a.repo.SaveUser(a.ctx, user)
	}

	jsonProfile, _ := a.profileManager.GetProfileByUserID(keys.UserID)
	if jsonProfile == nil {
		name := "User"
		avatarPath := ""
		if existingProfile != nil {
			name = existingProfile.Nickname
			avatarPath = existingProfile.Avatar
		}
		a.profileManager.CreateProfile(name, "", seedPhrase, keys.UserID, avatarPath, false, "")
	}

	go a.connectToI2P()
	return nil
}

// CreateAccount создаёт новый аккаунт
func (a *App) CreateAccount() (string, error) {
	id, err := identity.GenerateNewIdentity()
	if err != nil {
		return "", fmt.Errorf("failed to generate identity: %w", err)
	}

	a.identity = id

	if err := a.initUserRepository(id.Keys.UserID); err != nil {
		return "", fmt.Errorf("failed to init user repository: %w", err)
	}

	mc, err := media.NewMediaCrypt(id.Keys.EncryptionKey)
	if err == nil {
		a.mediaCrypt = mc
		mediaDir := filepath.Join(a.dataDir, "users", id.Keys.UserID, "media")
		os.MkdirAll(mediaDir, 0700)
	}

	user := &core.User{
		ID:         id.Keys.UserID,
		PublicKey:  id.Keys.PublicKeyBase64,
		PrivateKey: id.Keys.SigningPrivateKey,
		Mnemonic:   id.Mnemonic,
		Nickname:   "User",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	a.repo.SaveUser(a.ctx, user)

	go a.connectToI2P()
	return id.Mnemonic, nil
}

// Logout завершает сессию текущего пользователя
func (a *App) Logout() {
	a.identity = nil
	if a.repo != nil {
		a.repo.Close()
		a.repo = nil
	}
	if a.messenger != nil {
		a.messenger.Stop()
		a.messenger = nil
	}
	if a.router != nil {
		a.router.Stop()
		a.router = nil
	}
}

// GetMyInfo возвращает информацию о текущем пользователе
func (a *App) GetMyInfo() *UserInfo {
	if a.identity == nil {
		return nil
	}

	nickname := "User"
	avatar := ""

	if a.repo != nil {
		profile, err := a.repo.GetMyProfile(a.ctx)
		if err == nil && profile != nil {
			nickname = profile.Nickname
			avatar = profile.Avatar
		}
	}

	return &UserInfo{
		ID:          a.identity.Keys.UserID,
		Nickname:    nickname,
		Avatar:      avatar,
		PublicKey:   a.identity.Keys.PublicKeyBase64,
		Destination: a.GetMyDestination(),
		Fingerprint: a.identity.Keys.Fingerprint(),
	}
}

// UpdateMyProfile обновляет профиль
func (a *App) UpdateMyProfile(nickname, bio, avatar string) error {
	if err := a.repo.UpdateMyProfile(a.ctx, nickname, bio, avatar); err != nil {
		return err
	}
	go a.broadcastProfileUpdate()
	return nil
}

// BroadcastProfileUpdate broadcasts current profile to all contacts
func (a *App) broadcastProfileUpdate() {
	if a.messenger == nil {
		return
	}

	profile, err := a.repo.GetMyProfile(a.ctx)
	if err != nil || profile == nil {
		return
	}

	avatarBytes := []byte{}
	if profile.Avatar != "" {
		parts := strings.Split(profile.Avatar, ",")
		if len(parts) > 1 {
			avatarBytes, _ = base64.StdEncoding.DecodeString(parts[1])
		} else {
			avatarBytes, _ = base64.StdEncoding.DecodeString(profile.Avatar)
		}
	}

	packet := &pb.Packet{
		Type: pb.PacketType_PROFILE_UPDATE,
		Payload: func() []byte {
			update := &pb.ProfileUpdate{
				Nickname: profile.Nickname,
				Bio:      "",
				Avatar:   avatarBytes,
			}
			data, _ := proto.Marshal(update)
			return data
		}(),
	}

	a.messenger.Broadcast(packet)
}

func (a *App) onProfileUpdate(pubKey, nickname, bio string, avatar []byte) {
	if a.repo == nil {
		return
	}

	contact, err := a.repo.GetContactByPublicKey(a.ctx, pubKey)
	if err != nil || contact == nil {
		return
	}

	contact.Nickname = nickname
	if len(avatar) > 0 {
		contact.Avatar = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(avatar)
	}

	contact.UpdatedAt = time.Now()
	a.repo.SaveContact(a.ctx, contact)

	runtime.EventsEmit(a.ctx, "contact_updated", contact.ID)
}

// RequestProfileUpdate запрашивает обновление профиля у контакта
func (a *App) RequestProfileUpdate(contactID string) error {
	if a.messenger == nil {
		return fmt.Errorf("not connected to I2P")
	}

	contact, err := a.repo.GetContact(a.ctx, contactID)
	if err != nil || contact == nil {
		return fmt.Errorf("contact not found")
	}

	return a.messenger.SendProfileRequest(contact.I2PAddress)
}

// onProfileRequest обрабатывает входящий запрос профиля
func (a *App) onProfileRequest(requestorPubKey string) {
	a.broadcastProfileUpdate()
}
