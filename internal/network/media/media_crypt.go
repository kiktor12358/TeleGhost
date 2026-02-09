package media

import (
	"crypto/rand"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

// MediaCrypt управляет зашифрованным хранилищем медиафайлов
type MediaCrypt struct {
	key []byte // Мастер-ключ 32 байта
}

// NewMediaCrypt создает новый экземпляр MediaCrypt
func NewMediaCrypt(key []byte) (*MediaCrypt, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("invalid key size: expected 32 bytes")
	}
	return &MediaCrypt{key: key}, nil
}

// SaveEncrypted шифрует и сохраняет файл на диск
// Формат: [24 байта Nonce][Данные...]
func (m *MediaCrypt) SaveEncrypted(filename string, data []byte) error {
	aead, err := chacha20poly1305.NewX(m.key)
	if err != nil {
		return err
	}

	// Генерируем 24-байтный Nonce для XChaCha20
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	// Шифруем (Seal добавляет nonce как префикс)
	ciphertext := aead.Seal(nonce, nonce, data, nil)

	// Создаем директорию если нет
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}

	return os.WriteFile(filename, ciphertext, 0600)
}

// NewMediaHandler создает обработчик для AssetsHandler в Wails
func (m *MediaCrypt) NewMediaHandler(storageDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Перехватываем только GET запросы по префиксу /secure/
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/secure/") {
			http.NotFound(w, r)
			return
		}

		// Вычисляем путь к файлу на диске
		relPath := strings.TrimPrefix(r.URL.Path, "/secure/")
		fullPath := filepath.Join(storageDir, relPath)

		// Читаем зашифрованный файл
		encryptedData, err := os.ReadFile(fullPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		aead, err := chacha20poly1305.NewX(m.key)
		if err != nil {
			http.Error(w, "Encryption error", http.StatusInternalServerError)
			return
		}

		nonceSize := aead.NonceSize()
		if len(encryptedData) < nonceSize {
			http.Error(w, "Invalid media format", http.StatusForbidden)
			return
		}

		// Разделяем nonce и зашифрованные данные
		nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]

		// Расшифровываем в памяти
		plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			logError(fmt.Sprintf("Decryption failed for %s: %v", fullPath, err))
			http.Error(w, "Forbidden: Decryption failed", http.StatusForbidden)
			return
		}

		// Определяем Content-Type по расширению оригинального файла
		contentType := mime.TypeByExtension(filepath.Ext(fullPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(plaintext)))
		w.WriteHeader(http.StatusOK)
		w.Write(plaintext)
	})
}

// MigrateDirectory сканирует директорию и зашифровывает все файлы, которые еще не зашифрованы
func (m *MediaCrypt) MigrateDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		// Читаем файл
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // Пропускаем проблемные файлы
		}

		aead, err := chacha20poly1305.NewX(m.key)
		if err != nil {
			return err
		}

		// Проверяем, зашифрован ли уже файл
		nonceSize := aead.NonceSize()
		isEncrypted := false
		if len(data) >= nonceSize {
			nonce := data[:nonceSize]
			ciphertext := data[nonceSize:]
			_, errDec := aead.Open(nil, nonce, ciphertext, nil)
			if errDec == nil {
				isEncrypted = true
			}
		}

		if !isEncrypted {
			fmt.Printf("[MediaCrypt] Migrating to encrypted: %s\n", path)
			return m.SaveEncrypted(path, data)
		}

		return nil
	})
}

func logError(msg string) {
	// В реальном приложении здесь может быть логгер
	fmt.Printf("[MediaCrypt] %s\n", msg)
}
