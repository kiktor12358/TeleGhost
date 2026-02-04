// Package sqlite — тесты для SQLite репозитория
package sqlite

import (
	"context"
	"os"
	"testing"
	"time"

	"teleghost/internal/core"

	"github.com/google/uuid"
)

func setupTestDB(t *testing.T) (*Repository, func()) {
	t.Helper()

	// Создаём временный файл БД
	tmpFile, err := os.CreateTemp("", "teleghost_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Создаём репозиторий
	repo, err := New(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		t.Fatalf("Failed to create repository: %v", err)
	}

	// Выполняем миграции
	ctx := context.Background()
	if err := repo.Migrate(ctx); err != nil {
		repo.Close()
		os.Remove(tmpPath)
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Функция очистки
	cleanup := func() {
		repo.Close()
		os.Remove(tmpPath)
	}

	return repo, cleanup
}

func TestRepository_User(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Проверяем что профиля нет
	user, err := repo.GetMyProfile(ctx)
	if err != nil {
		t.Fatalf("GetMyProfile failed: %v", err)
	}
	if user != nil {
		t.Error("Expected nil user for empty database")
	}

	// Создаём пользователя
	newUser := &core.User{
		ID:         "test-user-id",
		PublicKey:  "dGVzdC1wdWJsaWMta2V5",
		PrivateKey: []byte("test-private-key"),
		Mnemonic:   "test mnemonic phrase",
		Nickname:   "TestUser",
		Bio:        "Test bio",
		I2PAddress: "test.i2p.address",
	}

	err = repo.SaveUser(ctx, newUser)
	if err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	// Получаем профиль
	user, err = repo.GetMyProfile(ctx)
	if err != nil {
		t.Fatalf("GetMyProfile failed: %v", err)
	}
	if user == nil {
		t.Fatal("Expected user, got nil")
	}

	if user.Nickname != "TestUser" {
		t.Errorf("Expected nickname 'TestUser', got '%s'", user.Nickname)
	}

	// Обновляем профиль
	err = repo.UpdateMyProfile(ctx, "NewNickname", "New bio", "avatar.png")
	if err != nil {
		t.Fatalf("UpdateMyProfile failed: %v", err)
	}

	// Проверяем обновление
	user, _ = repo.GetMyProfile(ctx)
	if user.Nickname != "NewNickname" {
		t.Errorf("Expected nickname 'NewNickname', got '%s'", user.Nickname)
	}

	t.Log("User tests passed")
}

func TestRepository_Contacts(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Создаём контакт
	contact := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  "contact-public-key",
		Nickname:   "Alice",
		Bio:        "Alice's bio",
		I2PAddress: "alice.i2p.address.base64",
		ChatID:     "chat-with-alice",
	}

	err := repo.SaveContact(ctx, contact)
	if err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	// Получаем контакт
	saved, err := repo.GetContact(ctx, contact.ID)
	if err != nil {
		t.Fatalf("GetContact failed: %v", err)
	}
	if saved == nil {
		t.Fatal("Expected contact, got nil")
	}
	if saved.Nickname != "Alice" {
		t.Errorf("Expected nickname 'Alice', got '%s'", saved.Nickname)
	}

	// Добавляем ещё контакт
	contact2 := &core.Contact{
		ID:         uuid.New().String(),
		PublicKey:  "bob-public-key",
		Nickname:   "Bob",
		I2PAddress: "bob.i2p.address.base64",
		ChatID:     "chat-with-bob",
	}
	repo.SaveContact(ctx, contact2)

	// Получаем список
	contacts, err := repo.ListContacts(ctx)
	if err != nil {
		t.Fatalf("ListContacts failed: %v", err)
	}
	if len(contacts) != 2 {
		t.Errorf("Expected 2 contacts, got %d", len(contacts))
	}

	// Удаляем контакт
	err = repo.DeleteContact(ctx, contact.ID)
	if err != nil {
		t.Fatalf("DeleteContact failed: %v", err)
	}

	// Проверяем удаление
	deleted, _ := repo.GetContact(ctx, contact.ID)
	if deleted != nil {
		t.Error("Expected nil after delete")
	}

	t.Log("Contact tests passed")
}

func TestRepository_Messages(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	chatID := "test-chat-id"

	// Создаём сообщения
	for i := 0; i < 5; i++ {
		msg := &core.Message{
			ID:          uuid.New().String(),
			ChatID:      chatID,
			SenderID:    "sender-id",
			Content:     "Test message " + string(rune('A'+i)),
			ContentType: "text",
			Status:      core.MessageStatusSent,
			IsOutgoing:  i%2 == 0,
			Timestamp:   time.Now().UnixMilli() + int64(i*1000),
		}

		err := repo.SaveMessage(ctx, msg)
		if err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}
	}

	// Получаем историю
	messages, err := repo.GetChatHistory(ctx, chatID, 10, 0)
	if err != nil {
		t.Fatalf("GetChatHistory failed: %v", err)
	}
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}

	// Проверяем порядок (DESC по timestamp)
	if messages[0].Content != "Test message E" {
		t.Errorf("Expected last message first, got: %s", messages[0].Content)
	}

	// Тест пагинации
	page2, _ := repo.GetChatHistory(ctx, chatID, 2, 2)
	if len(page2) != 2 {
		t.Errorf("Expected 2 messages on page 2, got %d", len(page2))
	}

	// Обновляем статус
	err = repo.UpdateMessageStatus(ctx, messages[0].ID, core.MessageStatusDelivered)
	if err != nil {
		t.Fatalf("UpdateMessageStatus failed: %v", err)
	}

	// Проверяем обновление статуса
	updated, _ := repo.GetMessage(ctx, messages[0].ID)
	if updated.Status != core.MessageStatusDelivered {
		t.Errorf("Expected status Delivered, got %d", updated.Status)
	}

	// Поиск
	found, err := repo.SearchMessages(ctx, chatID, "message C")
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("Expected 1 search result, got %d", len(found))
	}

	t.Log("Message tests passed")
}
