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
	Salt        string      `json:"salt"`
	Nonce       string      `json:"nonce"`
	Ciphertext  string      `json:"ciphertext"`
	ArgonParams ArgonParams `json:"argon_params"`
}

// ProfileMetadata метаданные профиля для отображения в списке
type ProfileMetadata struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
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

// CreateProfile создает новый зашифрованный профиль
func (pm *ProfileManager) CreateProfile(name string, pin string, mnemonic string) error {
	if len(pin) < 6 {
		return errors.New("ПИН-код слишком слабый")
	}

	id := uuid.New().String()
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}

	params := ArgonParams{
		Time:    4,
		Memory:  64 * 1024,
		Threads: 2,
	}

	// Генерация ключа из ПИН-кода
	key := argon2.IDKey([]byte(pin), salt, params.Time, params.Memory, params.Threads, chacha20poly1305.KeySize)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	ciphertext := aead.Seal(nil, nonce, []byte(mnemonic), nil)

	vault := Vault{
		ID:          id,
		DisplayName: name,
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
				profiles = append(profiles, ProfileMetadata{
					ID:          vault.ID,
					DisplayName: vault.DisplayName,
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
