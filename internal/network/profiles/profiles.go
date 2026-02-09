package profiles

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// ArgonParams параметры для Argon2id
type ArgonParams struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
}

// Vault контейнер с зашифрованными данными профиля
type Vault struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	UserID      string      `json:"user_id"`     // crypto ID
	AvatarPath  string      `json:"avatar_path"` // relative path
	UsePin      bool        `json:"use_pin"`     // if false, login by seed
	Salt        string      `json:"salt"`
	Nonce       string      `json:"nonce"`
	Ciphertext  string      `json:"ciphertext"`
	ArgonParams ArgonParams `json:"argon_params"`
}

// ProfileMetadata метаданные профиля для отображения в списке
type ProfileMetadata struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	AvatarPath  string `json:"avatar_path"`
	UsePin      bool   `json:"use_pin"`
}

// ProfileManager управляет профилями пользователей
type ProfileManager struct {
	storageDir string
}

// NewProfileManager создает новый менеджер профилей
func NewProfileManager(storageDir string) (*ProfileManager, error) {
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &ProfileManager{storageDir: storageDir}, nil
}

// CreateProfile создает новый зашифрованный профиль.
// Если передан existingID, используется он, иначе генерируется новый.
func (pm *ProfileManager) CreateProfile(name string, pin string, mnemonic string, userID string, avatarPath string, usePin bool, existingID string) error {
	if usePin && len(pin) < 6 {
		return errors.New("ПИН-код слишком слабый")
	}

	id := existingID
	if id == "" {
		id = uuid.New().String()
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}

	params := ArgonParams{
		Time:    4,
		Memory:  64 * 1024,
		Threads: 2,
	}

	var ciphertext []byte
	var nonce []byte

	// Если используем ПИН, шифруем мнемонику
	if usePin {
		// Генерация ключа из ПИН-кода
		key := argon2.IDKey([]byte(pin), salt, params.Time, params.Memory, params.Threads, chacha20poly1305.KeySize)

		aead, err := chacha20poly1305.NewX(key)
		if err != nil {
			return err
		}

		nonce = make([]byte, aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}

		ciphertext = aead.Seal(nil, nonce, []byte(mnemonic), nil)
	}

	// Копируем аватар если есть
	storedAvatarPath := ""
	if avatarPath != "" {
		ext := filepath.Ext(avatarPath)
		newAvatarName := id + "_avatar" + ext
		destPath := filepath.Join(pm.storageDir, newAvatarName)

		// Copy file
		input, err := os.ReadFile(avatarPath)
		if err == nil {
			if err := ioutil.WriteFile(destPath, input, 0600); err == nil {
				storedAvatarPath = newAvatarName
			}
		}
	}

	vault := Vault{
		ID:          id,
		DisplayName: name,
		UserID:      userID,
		AvatarPath:  storedAvatarPath,
		UsePin:      usePin,
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Nonce:       base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:  base64.StdEncoding.EncodeToString(ciphertext),
		ArgonParams: params,
	}

	data, err := json.MarshalIndent(vault, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(pm.storageDir, id+".json")
	return ioutil.WriteFile(filePath, data, 0600)
}

// GetProfileByUserID ищет профиль по UserID
func (pm *ProfileManager) GetProfileByUserID(userID string) (*ProfileMetadata, error) {
	profiles, err := pm.ListProfiles()
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if p.UserID == userID {
			return &p, nil
		}
	}
	return nil, nil // Not found
}

// UpdateProfile обновляет данные профиля
func (pm *ProfileManager) UpdateProfile(profileID string, name string, avatarPath string, deleteAvatar bool, usePin bool, newPin string, mnemonic string) error {
	filePath := filepath.Join(pm.storageDir, profileID+".json")
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("профиль не найден")
	}

	var vault Vault
	if err := json.Unmarshal(data, &vault); err != nil {
		return fmt.Errorf("ошибка чтения формата профиля")
	}

	// Обновляем имя
	if name != "" {
		vault.DisplayName = name
	}

	// Обновляем использование ПИН
	vault.UsePin = usePin

	// Обновляем аватар
	if deleteAvatar {
		// Удаляем старый если был
		if vault.AvatarPath != "" {
			os.Remove(filepath.Join(pm.storageDir, vault.AvatarPath))
			vault.AvatarPath = ""
		}
	} else if avatarPath != "" {
		// Удаляем старый если был
		if vault.AvatarPath != "" {
			os.Remove(filepath.Join(pm.storageDir, vault.AvatarPath))
		}

		ext := filepath.Ext(avatarPath)
		newAvatarName := profileID + "_avatar" + ext
		destPath := filepath.Join(pm.storageDir, newAvatarName)

		// Copy file
		input, err := os.ReadFile(avatarPath)
		if err == nil {
			if err := ioutil.WriteFile(destPath, input, 0600); err == nil {
				vault.AvatarPath = newAvatarName
			}
		}
	}

	// Если меняем ПИН или включаем его
	if usePin && newPin != "" {
		if mnemonic == "" {
			return fmt.Errorf("mnemonic required to change pin")
		}
		if len(newPin) < 6 {
			return errors.New("ПИН-код слишком слабый")
		}

		salt := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return err
		}

		vault.Salt = base64.StdEncoding.EncodeToString(salt)
		vault.ArgonParams = ArgonParams{
			Time:    4,
			Memory:  64 * 1024,
			Threads: 2,
		}

		key := argon2.IDKey([]byte(newPin), salt, vault.ArgonParams.Time, vault.ArgonParams.Memory, vault.ArgonParams.Threads, chacha20poly1305.KeySize)
		aead, err := chacha20poly1305.NewX(key)
		if err != nil {
			return err
		}

		nonce := make([]byte, aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}

		ciphertext := aead.Seal(nil, nonce, []byte(mnemonic), nil)
		vault.Nonce = base64.StdEncoding.EncodeToString(nonce)
		vault.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
	} else if !usePin {
		// Если ПИН отключен, очищаем крипто-поля (но userID остается)
		vault.Ciphertext = ""
		vault.Nonce = ""
		vault.Salt = ""
	}

	// Сохраняем
	newData, err := json.MarshalIndent(vault, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, newData, 0600)
}

// ListProfiles возвращает список доступных профилей
func (pm *ProfileManager) ListProfiles() ([]ProfileMetadata, error) {
	files, err := ioutil.ReadDir(pm.storageDir)
	if err != nil {
		return nil, err
	}

	var profiles []ProfileMetadata
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
			data, err := ioutil.ReadFile(filepath.Join(pm.storageDir, f.Name()))
			if err != nil {
				continue
			}

			var vault Vault
			if err := json.Unmarshal(data, &vault); err == nil {
				// Backward compatibility for existing profiles
				if vault.UserID == "" && vault.Ciphertext != "" {
					// We can't easily recover UserID without decrypting,
					// so existing profiles will rely on decryption to get ID.
					// Or we could have UI ask for pin to migrate.
				}

				avatarPath := ""
				if vault.AvatarPath != "" {
					avatarPath = filepath.Join(pm.storageDir, vault.AvatarPath)
				}

				profiles = append(profiles, ProfileMetadata{
					ID:          vault.ID,
					DisplayName: vault.DisplayName,
					UserID:      vault.UserID,
					AvatarPath:  avatarPath,
					UsePin:      vault.UsePin,
				})
			}
		}
	}

	return profiles, nil
}

// UnlockProfile проверяет ПИН и возвращает мнемонику
func (pm *ProfileManager) UnlockProfile(profileID string, pin string) (string, error) {
	filePath := filepath.Join(pm.storageDir, profileID+".json")
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("профиль не найден")
	}

	var vault Vault
	if err := json.Unmarshal(data, &vault); err != nil {
		return "", fmt.Errorf("ошибка чтения формата профиля")
	}

	if !vault.UsePin {
		// Если ПИН не используется, этот метод не должен вызываться для получения мнемоники,
		// так как она не хранится.
		// Но может быть старый профиль? Нет, новый флаг.
		return "", fmt.Errorf("profile does not use pin encryption")
	}

	salt, err := base64.StdEncoding.DecodeString(vault.Salt)
	if err != nil {
		return "", err
	}

	nonce, err := base64.StdEncoding.DecodeString(vault.Nonce)
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(vault.Ciphertext)
	if err != nil {
		return "", err
	}

	// Реконструкция ключа из ПИН-кода
	key := argon2.IDKey([]byte(pin), salt, vault.ArgonParams.Time, vault.ArgonParams.Memory, vault.ArgonParams.Threads, chacha20poly1305.KeySize)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return "", err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("Неверный ПИН")
	}

	return string(plaintext), nil
}
