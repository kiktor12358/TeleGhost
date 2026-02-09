package profiles

import (
	"os"
	"testing"
)

func TestProfileManager(t *testing.T) {
	tempDir := "test_profiles"
	defer os.RemoveAll(tempDir)

	pm, err := NewProfileManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create ProfileManager: %v", err)
	}

	name := "Test User"
	pin := "123456"
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// 1. Test Weak PIN
	err = pm.CreateProfile(name, "12345", mnemonic, "user-id-123", "", true, "")
	if err == nil || err.Error() != "ПИН-код слишком слабый" {
		t.Errorf("Expected error for weak PIN, got: %v", err)
	}

	// Test CreateProfile
	err = pm.CreateProfile("Test User", "123456", mnemonic, "user-id-123", "", true, "")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	// Test ListProfiles
	profiles, err := pm.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles failed: %v", err)
	}
	if len(profiles) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].DisplayName != name {
		t.Errorf("Expected name %s, got %s", name, profiles[0].DisplayName)
	}

	profileID := profiles[0].ID

	// 4. Test Unlock with Wrong PIN
	_, err = pm.UnlockProfile(profileID, "654321")
	if err == nil || err.Error() != "Неверный ПИН" {
		t.Errorf("Expected 'Неверный ПИН' error, got: %v", err)
	}

	// 5. Test Unlock with Correct PIN
	decrypted, err := pm.UnlockProfile(profileID, pin)
	if err != nil {
		t.Fatalf("Failed to unlock profile: %v", err)
	}
	if decrypted != mnemonic {
		t.Errorf("Expected mnemonic %s, got %s", mnemonic, decrypted)
	}
}
