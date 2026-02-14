package media

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestMediaCrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	mc, err := NewMediaCrypt(key)
	if err != nil {
		t.Fatalf("Failed to create MediaCrypt: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "mediacrypt_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Test Save and Encryption
	originalData := []byte("hello world encryption test")
	testFile := filepath.Join(tempDir, "test.txt")

	err = mc.SaveEncrypted(testFile, originalData)
	if err != nil {
		t.Fatalf("Failed to save encrypted: %v", err)
	}

	// Read raw data to check if it's different
	encryptedData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read encrypted file: %v", err)
	}
	if bytes.Equal(originalData, encryptedData) {
		t.Error("Encrypted data is same as original")
	}

	// 2. Test Migration
	plainFile := filepath.Join(tempDir, "plain.txt")
	plainData := []byte("this is plain text")
	err = os.WriteFile(plainFile, plainData, 0644)
	if err != nil {
		t.Fatalf("Failed to write plain file: %v", err)
	}

	err = mc.MigrateDirectory(tempDir)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify plain file is now encrypted
	migratedData, err := os.ReadFile(plainFile)
	if err != nil {
		t.Fatalf("Failed to read migrated file: %v", err)
	}
	if bytes.Equal(plainData, migratedData) {
		t.Error("Plain file was not migrated (still same)")
	}

	// 3. Verify it can be decrypted (via logic in MigrateDirectory or manual)
	aead, _ := chacha20poly1305.NewX(key)
	nonceSize := aead.NonceSize()
	if len(migratedData) < nonceSize {
		t.Fatal("Migrated data too short")
	}
	nonce, ciphertext := migratedData[:nonceSize], migratedData[nonceSize:]
	decrypted, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("Failed to decrypt migrated data: %v", err)
	}
	if !bytes.Equal(plainData, decrypted) {
		t.Fatal("Decrypted data does not match original plain data")
	}

	// 4. Test DecryptDirectory
	err = mc.DecryptDirectory(tempDir)
	if err != nil {
		t.Fatalf("DecryptDirectory failed: %v", err)
	}

	// Verify file is now plaintext on disk
	onDiskData, err := os.ReadFile(plainFile)
	if err != nil {
		t.Fatalf("Failed to read decrypted file: %v", err)
	}
	if !bytes.Equal(plainData, onDiskData) {
		t.Error("File on disk is not decrypted after DecryptDirectory")
	}
}
