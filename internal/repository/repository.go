// Package repository предоставляет интерфейсы и реализации для работы с хранилищем данных
package repository

import (
	"context"

	"teleghost/internal/core"
)

// UserRepository определяет операции с пользователем
type UserRepository interface {
	// GetUser возвращает текущего пользователя
	GetUser(ctx context.Context) (*core.User, error)

	// SaveUser сохраняет/обновляет пользователя
	SaveUser(ctx context.Context, user *core.User) error

	// UpdateProfile обновляет профиль пользователя
	UpdateProfile(ctx context.Context, nickname, bio, avatar string) error
}

// ContactRepository определяет операции с контактами
type ContactRepository interface {
	// GetContact возвращает контакт по ID
	GetContact(ctx context.Context, id string) (*core.Contact, error)

	// GetContactByPublicKey ищет контакт по публичному ключу
	GetContactByPublicKey(ctx context.Context, pubKey string) (*core.Contact, error)

	// ListContacts возвращает список всех контактов
	ListContacts(ctx context.Context) ([]*core.Contact, error)

	// ListContactsWithLastMessage возвращает контакты с их последним сообщением
	ListContactsWithLastMessage(ctx context.Context) ([]*core.Contact, error)

	// SaveContact сохраняет новый контакт
	SaveContact(ctx context.Context, contact *core.Contact) error

	// UpdateContact обновляет существующий контакт
	UpdateContact(ctx context.Context, contact *core.Contact) error

	// DeleteContact удаляет контакт
	DeleteContact(ctx context.Context, id string) error

	// BlockContact блокирует/разблокирует контакт
	BlockContact(ctx context.Context, id string, blocked bool) error
}

// MessageRepository определяет операции с сообщениями
type MessageRepository interface {
	// GetMessage возвращает сообщение по ID
	GetMessage(ctx context.Context, id string) (*core.Message, error)

	// ListMessages возвращает сообщения чата с пагинацией
	ListMessages(ctx context.Context, chatID string, limit, offset int) ([]*core.Message, error)

	// SaveMessage сохраняет новое сообщение
	SaveMessage(ctx context.Context, msg *core.Message) error

	// UpdateMessageStatus обновляет статус доставки
	UpdateMessageStatus(ctx context.Context, id string, status core.MessageStatus) error

	// DeleteMessage удаляет сообщение
	DeleteMessage(ctx context.Context, id string) error

	// SearchMessages ищет сообщения по тексту
	SearchMessages(ctx context.Context, chatID, query string) ([]*core.Message, error)
}

// ChatRepository определяет операции с чатами
type ChatRepository interface {
	// GetChat возвращает чат по ID
	GetChat(ctx context.Context, id string) (*core.Chat, error)

	// GetChatByContactID возвращает чат по ID контакта
	GetChatByContactID(ctx context.Context, contactID string) (*core.Chat, error)

	// ListChats возвращает список всех чатов
	ListChats(ctx context.Context) ([]*core.Chat, error)

	// SaveChat создаёт новый чат
	SaveChat(ctx context.Context, chat *core.Chat) error

	// UpdateChat обновляет чат
	UpdateChat(ctx context.Context, chat *core.Chat) error

	// MarkAsRead помечает сообщения как прочитанные
	MarkAsRead(ctx context.Context, chatID string) error
}

// Repository объединяет все репозитории
type Repository interface {
	UserRepository
	ContactRepository
	MessageRepository
	ChatRepository

	// Close закрывает соединение с базой данных
	Close() error

	// Migrate выполняет миграции базы данных
	Migrate(ctx context.Context) error
}
