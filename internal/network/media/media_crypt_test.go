package media

import (
	"bytes"
	"io/ioutil"
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

	tempDir, err := ioutil.TempDir("", "mediacrypt_test")
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
	encryptedData, _ := ioutil.ReadFile(testFile)
	if bytes.Equal(originalData, encryptedData) {
		t.Error("Encrypted data is same as original")
	}

	// 2. Test Migration
	plainFile := filepath.Join(tempDir, "plain.txt")
	plainData := []byte("this is plain text")
	ioutil.WriteFile(plainFile, plainData, 0644)

	err = mc.MigrateDirectory(tempDir)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify plain file is now encrypted
	migratedData, _ := ioutil.ReadFile(plainFile)
	if bytes.Equal(plainData, migratedData) {
		t.Error("Plain file was not migrated (still same)")
	}

	// 3. Verify it can be decrypted (via logic in MigrateDirectory or manual)
	// We'll trust the logic if the app runs, but let's do one manual check
	aead, _ := chacha20poly1305.NewX(key)
	nonceSize := aead.NonceSize()
	nonce, ciphertext := migratedData[:nonceSize], migratedData[nonceSize:]
	decrypted, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("Failed to decrypt migrated data: %v", err)
	}
	if !bytes.Equal(plainData, decrypted) {
		t.Fatal("Decrypted data does not match original plain data")
	}
}
