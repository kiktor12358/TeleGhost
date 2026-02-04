// Package identity реализует криптографическую идентичность пользователя
// на основе BIP-39 мнемонической фразы
package identity

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// MnemonicBits — количество бит энтропии (128 = 12 слов, 256 = 24 слова)
	MnemonicBits = 128

	// Ed25519SeedSize — размер seed для Ed25519
	Ed25519SeedSize = 32

	// ChaCha20KeySize — размер ключа ChaCha20-Poly1305
	ChaCha20KeySize = chacha20poly1305.KeySize
)

// Keys содержит все криптографические ключи пользователя
type Keys struct {
	// SigningPrivateKey — приватный ключ Ed25519 для подписи сообщений
	SigningPrivateKey ed25519.PrivateKey

	// SigningPublicKey — публичный ключ Ed25519
	SigningPublicKey ed25519.PublicKey

	// EncryptionKey — ключ для шифрования локальной БД (ChaCha20-Poly1305)
	EncryptionKey []byte

	// PublicKeyBase64 — публичный ключ в base64 (для удобства)
	PublicKeyBase64 string

	// UserID — уникальный ID пользователя (первые 16 байт SHA256 от публичного ключа)
	UserID string
}

// Identity представляет криптографическую идентичность пользователя
type Identity struct {
	// Mnemonic — BIP-39 мнемоническая фраза (12 слов)
	Mnemonic string

	// Keys — криптографические ключи
	Keys *Keys
}

// GenerateNewIdentity создаёт новую идентичность с случайной мнемоникой
func GenerateNewIdentity() (*Identity, error) {
	// Генерируем энтропию
	entropy, err := bip39.NewEntropy(MnemonicBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate entropy: %w", err)
	}

	// Создаём мнемонику из энтропии
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	// Восстанавливаем ключи из мнемоники
	keys, err := RecoverKeys(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("failed to derive keys: %w", err)
	}

	return &Identity{
		Mnemonic: mnemonic,
		Keys:     keys,
	}, nil
}

// RecoverKeys детерминировано восстанавливает все ключи из мнемоники
func RecoverKeys(mnemonic string) (*Keys, error) {
	// Валидируем мнемонику
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid mnemonic phrase")
	}

	// Получаем seed из мнемоники (используем пустой passphrase)
	seed := bip39.NewSeed(mnemonic, "")

	// Используем HKDF для детерминированного вывода различных ключей
	// Info разделяет ключи для разных целей

	// 1. Ключ для подписи (Ed25519)
	signingKey, err := deriveKey(seed, "teleghost-signing-key-v1", Ed25519SeedSize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive signing key: %w", err)
	}

	// Создаём Ed25519 ключевую пару из seed
	privateKey := ed25519.NewKeyFromSeed(signingKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// 2. Ключ для шифрования БД (ChaCha20-Poly1305)
	encryptionKey, err := deriveKey(seed, "teleghost-db-encryption-key-v1", ChaCha20KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	// Генерируем UserID из публичного ключа
	userID := generateUserID(publicKey)

	return &Keys{
		SigningPrivateKey: privateKey,
		SigningPublicKey:  publicKey,
		EncryptionKey:     encryptionKey,
		PublicKeyBase64:   base64.StdEncoding.EncodeToString(publicKey),
		UserID:            userID,
	}, nil
}

// deriveKey выводит ключ указанной длины из master seed используя HKDF
func deriveKey(masterSeed []byte, info string, keyLen int) ([]byte, error) {
	// Используем SHA-512 для HKDF
	hkdfReader := hkdf.New(sha512.New, masterSeed, nil, []byte(info))

	key := make([]byte, keyLen)
	if _, err := hkdfReader.Read(key); err != nil {
		return nil, err
	}

	return key, nil
}

// generateUserID создаёт уникальный ID из публичного ключа
func generateUserID(publicKey ed25519.PublicKey) string {
	hash := sha256.Sum256(publicKey)
	// Используем первые 16 байт в base64 = 22 символа
	return base64.RawURLEncoding.EncodeToString(hash[:16])
}

// SignMessage подписывает сообщение приватным ключом
func (k *Keys) SignMessage(message []byte) []byte {
	return ed25519.Sign(k.SigningPrivateKey, message)
}

// VerifySignature проверяет подпись сообщения публичным ключом
func VerifySignature(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return ed25519.Verify(publicKey, message, signature)
}

// VerifySignatureBase64 проверяет подпись с публичным ключом в base64
func VerifySignatureBase64(publicKeyBase64 string, message, signature []byte) (bool, error) {
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return false, fmt.Errorf("invalid public key base64: %w", err)
	}

	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key size: %d", len(publicKeyBytes))
	}

	return ed25519.Verify(publicKeyBytes, message, signature), nil
}

// Fingerprint возвращает короткий отпечаток публичного ключа для верификации
func (k *Keys) Fingerprint() string {
	hash := sha256.Sum256(k.SigningPublicKey)
	// Возвращаем первые 8 байт в hex формате с разделителями
	return fmt.Sprintf("%X:%X:%X:%X",
		hash[0:2], hash[2:4], hash[4:6], hash[6:8])
}

// ValidateMnemonic проверяет валидность мнемонической фразы
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}
