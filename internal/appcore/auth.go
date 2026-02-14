package appcore

import (
	"fmt"
	"log"
	"os"
	"strings"
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
			"avatar_path":  a.formatProfileAvatarURL(p.UserID, p.AvatarPath),
			"use_pin":      p.UsePin,
		}
	}
	return result, nil
}

// CreateProfile создаёт новый зашифрованный профиль.
func (a *AppCore) CreateProfile(name, pin, mnemonic, existingUserID, avatarPath string, usePin bool) error {
	log.Printf("[AppCore] Creating/Updating profile for: %s", name)

	var userID string
	if existingUserID != "" {
		userID = existingUserID
	} else {
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

			// Если есть аватар в ПМ, импортируем его в папку пользователя
			finalAvatar := avatar
			if avatar != "" {
				if data, err := os.ReadFile(avatar); err == nil {
					if savedPath, err := a.SaveAvatar("my_avatar.png", data); err == nil {
						finalAvatar = savedPath
					}
				}
			}

			user := &core.User{
				ID:        keys.UserID,
				PublicKey: keys.PublicKeyBase64,
				Nickname:  nickname,
				Avatar:    finalAvatar,
				UpdatedAt: time.Now(),
			}
			if err := a.Repo.SaveUser(a.Ctx, user); err != nil {
				log.Printf("[Auth] Failed to save user to DB: %v", err)
			}
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
				dbUserNickname := dbUser.Nickname
				nickname = dbUserNickname
				needsUpdatePM = true
			}

			// То же самое для аватара
			// То же самое для аватара
			// Логика синхронизации:
			// 1. Если в БД нет аватара, а в ПМ есть -> Импортируем из ПМ в users/... и обновляем БД
			if dbUser.Avatar == "" && avatar != "" {
				if data, err := os.ReadFile(avatar); err == nil {
					// SaveAvatar сохраняет в папку пользователя и возвращает абсолютный путь
					if savedPath, err := a.SaveAvatar("my_avatar.png", data); err == nil {
						dbUser.Avatar = savedPath
						needsUpdateDB = true
					}
				}
			} else if dbUser.Avatar != "" && avatar == "" {
				// 2. Если в БД есть, а в ПМ нет -> Обновляем ПМ
				dbUserAvatar := dbUser.Avatar
				avatar = dbUserAvatar
				needsUpdatePM = true
			} else if dbUser.Avatar != "" && avatar != "" && dbUser.Avatar != avatar {
				// 3. Если есть в обоих местах, но пути разные.
				// ProfileManager создает свою копию (путь .../profiles/...), а БД хранит путь (.../users/...).
				// Они всегда будут отличаться путями.
				// Мы должны доверять БД, если путь выглядит как "правильный" (лежит в users/.../avatars).

				isDbInternal := strings.Contains(dbUser.Avatar, "users") && strings.Contains(dbUser.Avatar, "avatars")

				if !isDbInternal {
					// Если в БД какой-то левый путь, то берем из ПМ и импортируем
					if data, err := os.ReadFile(avatar); err == nil {
						if savedPath, err := a.SaveAvatar("my_avatar.png", data); err == nil {
							dbUser.Avatar = savedPath
							needsUpdateDB = true
						}
					}
				}
				// Если isDbInternal, то оставляем как есть в БД.
			}

			if needsUpdateDB {
				log.Printf("[Auth] Syncing DB nickname: %s", dbUser.Nickname)
				if err := a.Repo.UpdateMyProfile(a.Ctx, dbUser.Nickname, dbUser.Bio, dbUser.Avatar); err != nil {
					log.Printf("[Auth] Failed to sync DB profile: %v", err)
				}
			}
			if needsUpdatePM && a.ProfileManager != nil {
				if meta, _ := a.ProfileManager.GetProfileByUserID(keys.UserID); meta != nil {
					log.Printf("[Auth] Syncing PM nickname: %s", dbUser.Nickname)
					if err := a.UpdateMyProfile(dbUser.Nickname, dbUser.Bio, dbUser.Avatar); err != nil {
						log.Printf("[Auth] Failed to sync PM profile: %v", err)
					}
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
		_ = a.Messenger.Stop()
		a.Messenger = nil
	}
	if a.Router != nil {
		_ = a.Router.Stop()
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

	res["Status"] = string(a.Status)

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
