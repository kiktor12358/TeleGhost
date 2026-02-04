// Package identity — тесты для криптографической идентичности
package identity

import (
	"bytes"
	"testing"
)

func TestGenerateNewIdentity(t *testing.T) {
	identity, err := GenerateNewIdentity()
	if err != nil {
		t.Fatalf("GenerateNewIdentity failed: %v", err)
	}

	// Проверяем что мнемоника валидна
	if !ValidateMnemonic(identity.Mnemonic) {
		t.Error("Generated mnemonic is invalid")
	}

	// Проверяем что ключи сгенерированы
	if identity.Keys == nil {
		t.Fatal("Keys are nil")
	}

	if len(identity.Keys.SigningPrivateKey) != 64 {
		t.Errorf("Invalid private key length: %d", len(identity.Keys.SigningPrivateKey))
	}

	if len(identity.Keys.SigningPublicKey) != 32 {
		t.Errorf("Invalid public key length: %d", len(identity.Keys.SigningPublicKey))
	}

	if len(identity.Keys.EncryptionKey) != 32 {
		t.Errorf("Invalid encryption key length: %d", len(identity.Keys.EncryptionKey))
	}

	if identity.Keys.UserID == "" {
		t.Error("UserID is empty")
	}

	t.Logf("Generated identity:")
	t.Logf("  Mnemonic: %s", identity.Mnemonic)
	t.Logf("  UserID: %s", identity.Keys.UserID)
	t.Logf("  PublicKey: %s", identity.Keys.PublicKeyBase64)
	t.Logf("  Fingerprint: %s", identity.Keys.Fingerprint())
}

func TestRecoverKeys_Deterministic(t *testing.T) {
	// Тестовая мнемоника (НЕ использовать в продакшене!)
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Восстанавливаем ключи дважды
	keys1, err := RecoverKeys(mnemonic)
	if err != nil {
		t.Fatalf("RecoverKeys failed (1): %v", err)
	}

	keys2, err := RecoverKeys(mnemonic)
	if err != nil {
		t.Fatalf("RecoverKeys failed (2): %v", err)
	}

	// Проверяем детерминированность
	if !bytes.Equal(keys1.SigningPrivateKey, keys2.SigningPrivateKey) {
		t.Error("Private keys are not deterministic")
	}

	if !bytes.Equal(keys1.SigningPublicKey, keys2.SigningPublicKey) {
		t.Error("Public keys are not deterministic")
	}

	if !bytes.Equal(keys1.EncryptionKey, keys2.EncryptionKey) {
		t.Error("Encryption keys are not deterministic")
	}

	if keys1.UserID != keys2.UserID {
		t.Error("UserIDs are not deterministic")
	}

	t.Logf("Deterministic recovery verified for mnemonic")
	t.Logf("  UserID: %s", keys1.UserID)
}

func TestRecoverKeys_InvalidMnemonic(t *testing.T) {
	_, err := RecoverKeys("invalid mnemonic phrase")
	if err == nil {
		t.Error("RecoverKeys should fail for invalid mnemonic")
	}
}

func TestSignAndVerify(t *testing.T) {
	identity, err := GenerateNewIdentity()
	if err != nil {
		t.Fatalf("GenerateNewIdentity failed: %v", err)
	}

	message := []byte("Hello, TeleGhost!")

	// Подписываем сообщение
	signature := identity.Keys.SignMessage(message)

	// Проверяем подпись
	if !VerifySignature(identity.Keys.SigningPublicKey, message, signature) {
		t.Error("Signature verification failed")
	}

	// Проверяем что подпись не проходит для другого сообщения
	if VerifySignature(identity.Keys.SigningPublicKey, []byte("wrong message"), signature) {
		t.Error("Signature should not verify for different message")
	}

	t.Logf("Sign/Verify test passed")
}

func TestVerifySignatureBase64(t *testing.T) {
	identity, err := GenerateNewIdentity()
	if err != nil {
		t.Fatalf("GenerateNewIdentity failed: %v", err)
	}

	message := []byte("Test message for base64 verification")
	signature := identity.Keys.SignMessage(message)

	// Проверяем через base64
	valid, err := VerifySignatureBase64(identity.Keys.PublicKeyBase64, message, signature)
	if err != nil {
		t.Fatalf("VerifySignatureBase64 failed: %v", err)
	}

	if !valid {
		t.Error("Base64 signature verification failed")
	}
}

func TestFingerprint(t *testing.T) {
	identity, err := GenerateNewIdentity()
	if err != nil {
		t.Fatalf("GenerateNewIdentity failed: %v", err)
	}

	fingerprint := identity.Keys.Fingerprint()

	// Формат: XX:XX:XX:XX (4 группы по 2 байта в hex)
	if len(fingerprint) != 19 { // 4*4 + 3 двоеточия
		t.Errorf("Invalid fingerprint format: %s (len=%d)", fingerprint, len(fingerprint))
	}

	t.Logf("Fingerprint: %s", fingerprint)
}
