package appcore

import (
	"fmt"
	"log"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
)

// ListProfiles возвращает список доступных профилей.
func (a *AppCore) ListProfiles() ([]map[string]interface{}, error) {
	if a.ProfileManager == nil {
		return nil, fmt.Errorf("profile manager not initialized")
	}
	profiles, err := a.ProfileManager.ListProfiles()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(profiles))
	for i, p := range profiles {
		result[i] = map[string]interface{}{
			"id":           p.ID,
			"display_name": p.DisplayName,
			"user_id":      p.UserID,
			"avatar_path":  p.AvatarPath,
			"use_pin":      p.UsePin,
		}
	}
	return result, nil
}

// CreateProfile создаёт новый зашифрованный профиль.
func (a *AppCore) CreateProfile(name, pin, mnemonic, userID, avatarPath string, usePin bool) error {
	if a.ProfileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}

	// Если мнемоника не передана, генерируем новую
	if mnemonic == "" {
		newID, err := identity.GenerateNewIdentity()
		if err != nil {
			return err
		}
		mnemonic = newID.Mnemonic
		userID = newID.Keys.UserID
	} else {
		// Если передана, восстанавливаем ключи для проверки и получения userID
		keys, err := identity.RecoverKeys(mnemonic)
		if err != nil {
			return fmt.Errorf("invalid mnemonic: %w", err)
		}
		userID = keys.UserID
	}

	return a.ProfileManager.CreateProfile(name, pin, mnemonic, userID, avatarPath, usePin, "")
}

// UnlockProfile проверяет PIN и возвращает мнемонику.
func (a *AppCore) UnlockProfile(profileID, pin string) (string, error) {
	if a.ProfileManager == nil {
		return "", fmt.Errorf("profile manager not initialized")
	}
	return a.ProfileManager.UnlockProfile(profileID, pin)
}

// Login авторизует пользователя.
func (a *AppCore) Login(mnemonic string) error {
	keys, err := identity.RecoverKeys(mnemonic)
	if err != nil {
		return fmt.Errorf("failed to recover keys: %w", err)
	}

	a.Identity = &identity.Identity{
		Mnemonic: mnemonic,
		Keys:     keys,
	}

	if err := a.InitUserRepository(keys.UserID); err != nil {
		return err
	}

	// Сохраняем/обновляем профиль в БД
	if a.Repo != nil {
		// Получаем актуальные данные из ProfileManager
		nickname := ""
		avatar := ""
		if a.ProfileManager != nil {
			if meta, _ := a.ProfileManager.GetProfileByUserID(keys.UserID); meta != nil {
				nickname = meta.DisplayName
				avatar = meta.AvatarPath
			}
		}

		// Проверяем, есть ли уже такой пользователь в БД
		dbUser, err := a.Repo.GetMyProfile(a.Ctx)
		if err != nil || dbUser == nil {
			// Если нет в БД, создаем с именем из ProfileManager или дефолтным
			if nickname == "" {
				nickname = "User"
			}
			user := &core.User{
				ID:        keys.UserID,
				PublicKey: keys.PublicKeyBase64,
				Nickname:  nickname,
				Avatar:    avatar,
				UpdatedAt: time.Now(),
			}
			a.Repo.SaveUser(a.Ctx, user)
		} else {
			// Если есть в БД, синхронизируем данные
			needsUpdateDB := false
			needsUpdatePM := false

			// Если в ПМ есть имя и оно не "User", а в БД другое - обновляем БД
			if nickname != "" && nickname != "User" && dbUser.Nickname != nickname {
				dbUser.Nickname = nickname
				needsUpdateDB = true
			} else if (dbUser.Nickname != "" && dbUser.Nickname != "User") && (nickname == "" || nickname == "User") {
				// Если в БД есть имя, а в ПМ нет или дефолтное - обновляем ПМ (синхронизация в обратную сторону)
				nickname = dbUser.Nickname
				needsUpdatePM = true
			}

			// То же самое для аватара
			if avatar != "" && dbUser.Avatar != avatar {
				dbUser.Avatar = avatar
				needsUpdateDB = true
			} else if dbUser.Avatar != "" && avatar == "" {
				avatar = dbUser.Avatar
				needsUpdatePM = true
			}

			if needsUpdateDB {
				log.Printf("[Auth] Syncing DB nickname: %s", dbUser.Nickname)
				a.Repo.UpdateMyProfile(a.Ctx, dbUser.Nickname, dbUser.Bio, dbUser.Avatar)
			}
			if needsUpdatePM && a.ProfileManager != nil {
				if meta, _ := a.ProfileManager.GetProfileByUserID(keys.UserID); meta != nil {
					log.Printf("[Auth] Syncing PM nickname: %s", dbUser.Nickname)
					a.UpdateMyProfile(dbUser.Nickname, dbUser.Bio, dbUser.Avatar)
				}
			}
		}
	}

	// Подключаемся к сети
	go a.ConnectToI2P()

	return nil
}

// CreateAccount создаёт новый аккаунт.
func (a *AppCore) CreateAccount() (string, error) {
	id, err := identity.GenerateNewIdentity()
	if err != nil {
		return "", err
	}

	a.Identity = id

	return id.Mnemonic, nil
}

// Logout завершает сессию.
func (a *AppCore) Logout() {
	log.Printf("[AppCore] Logging out...")

	if a.Messenger != nil {
		a.Messenger.Stop()
		a.Messenger = nil
	}
	if a.Router != nil {
		a.Router.Stop()
		a.Router = nil
	}
	if a.Repo != nil {
		a.Repo.Close()
		a.Repo = nil
	}

	a.Identity = nil
	a.SetNetworkStatus(StatusOffline)
}

// GetMyInfo returns information about the current user
func (a *AppCore) GetMyInfo() map[string]interface{} {
	if a.Identity == nil {
		return nil
	}

	res := map[string]interface{}{
		"ID":        a.Identity.Keys.UserID,
		"PublicKey": a.Identity.Keys.PublicKeyBase64,
		"Mnemonic":  a.Identity.Mnemonic, // Return mnemonic for privacy settings
	}

	if a.Repo != nil {
		if u, err := a.Repo.GetMyProfile(a.Ctx); err == nil && u != nil {
			res["Nickname"] = u.Nickname
			res["Avatar"] = a.formatAvatarURL(u.Avatar)
			res["Bio"] = u.Bio
		}
	}

	if a.Messenger != nil {
		res["Destination"] = a.Messenger.GetDestination()
	} else if a.Router != nil {
		res["Destination"] = a.Router.GetDestination()
	}

	return res
}

// DeleteProfile удаляет профиль.
func (a *AppCore) DeleteProfile(profileID string) error {
	if a.ProfileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}
	return a.ProfileManager.DeleteProfile(profileID)
}

// UpdateProfile обновляет данные зашифрованного профиля (используется для ПИН-кода).
func (a *AppCore) UpdateProfile(profileID, name, avatarPath string, deleteAvatar bool, usePin bool, newPin, mnemonic string) error {
	if a.ProfileManager == nil {
		return fmt.Errorf("profile manager not initialized")
	}
	return a.ProfileManager.UpdateProfile(profileID, name, avatarPath, deleteAvatar, usePin, newPin, mnemonic)
}
