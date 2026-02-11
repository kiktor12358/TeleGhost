// Package core содержит бизнес-логику и модели данных TeleGhost
package core

import (
	"time"
)

// User представляет текущего пользователя (владельца приложения)
type User struct {
	// ID — уникальный идентификатор, производный от публичного ключа
	ID string `json:"id" db:"id"`

	// PublicKey — публичный ключ Ed25519 (32 bytes, base64)
	PublicKey string `json:"public_key" db:"public_key"`

	// PrivateKey — приватный ключ Ed25519 (64 bytes, зашифрован)
	// Не сериализуется в JSON!
	PrivateKey []byte `json:"-" db:"private_key"`

	// Mnemonic — BIP-39 мнемоническая фраза (зашифрована)
	// Используется для восстановления ключей
	Mnemonic string `json:"-" db:"mnemonic"`

	// Nickname — отображаемое имя
	Nickname string `json:"nickname" db:"nickname"`

	// Bio — краткое описание профиля
	Bio string `json:"bio" db:"bio"`

	// Avatar — путь к файлу аватара или base64
	Avatar string `json:"avatar" db:"avatar"`

	// I2PAddress — полный I2P destination address (base64, ~516 chars)
	I2PAddress string `json:"i2p_address" db:"i2p_address"`

	// I2PKeys — приватные ключи I2P (зашифрованы, хранятся как байты)
	I2PKeys []byte `json:"-" db:"i2p_keys"`

	// CreatedAt — время создания аккаунта
	CreatedAt time.Time `json:"created_at" db:"created_at"`

	// UpdatedAt — время последнего обновления профиля
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Contact представляет контакт (друга) в адресной книге
type Contact struct {
	// ID — уникальный идентификатор контакта
	ID string `json:"id" db:"id"`

	// PublicKey — публичный ключ контакта Ed25519 (base64)
	PublicKey string `json:"public_key" db:"public_key"`

	// Nickname — отображаемое имя контакта
	Nickname string `json:"nickname" db:"nickname"`

	// Bio — описание профиля контакта
	Bio string `json:"bio" db:"bio"`

	// Avatar — аватар контакта (base64 или путь)
	Avatar string `json:"avatar" db:"avatar"`

	// I2PAddress — I2P destination адрес контакта
	// Длинная строка base64 (~516 символов), полный I2P destination
	I2PAddress string `json:"i2p_address" db:"i2p_address"`

	// ChatID — ID чата с этим контактом
	ChatID string `json:"chat_id" db:"chat_id"`

	// IsBlocked — заблокирован ли контакт
	IsBlocked bool `json:"is_blocked" db:"is_blocked"`

	// IsVerified — подтверждён ли контакт (fingerprint check)
	IsVerified bool `json:"is_verified" db:"is_verified"`

	// LastSeen — время последней активности контакта
	LastSeen *time.Time `json:"last_seen,omitempty" db:"last_seen"`

	// AddedAt — когда контакт был добавлен
	AddedAt time.Time `json:"added_at" db:"added_at"`

	// UpdatedAt — последнее обновление информации о контакте
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`

	// LastMessage — последнее сообщение (не хранится в этой таблице)
	LastMessage string `json:"last_message,omitempty" db:"-"`
	// LastMessageTime — время последнего сообщения
	LastMessageTime time.Time `json:"last_message_time,omitempty" db:"-"`
}

// MessageStatus определяет статус доставки сообщения
type MessageStatus int

const (
	// MessageStatusPending — сообщение отправляется
	MessageStatusPending MessageStatus = iota
	// MessageStatusSent — сообщение отправлено в сеть
	MessageStatusSent
	// MessageStatusDelivered — сообщение доставлено получателю
	MessageStatusDelivered
	// MessageStatusRead — сообщение прочитано
	MessageStatusRead
	// MessageStatusFailed — ошибка отправки
	MessageStatusFailed
)

// String возвращает строковое представление статуса
func (s MessageStatus) String() string {
	switch s {
	case MessageStatusPending:
		return "pending"
	case MessageStatusSent:
		return "sent"
	case MessageStatusDelivered:
		return "delivered"
	case MessageStatusRead:
		return "read"
	case MessageStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ReplyPreview содержит краткую информацию об исходном сообщении для ответа
type ReplyPreview struct {
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
}

// Message представляет сообщение в чате
type Message struct {
	// ID — уникальный идентификатор сообщения (UUID)
	ID string `json:"id" db:"id"`

	// ChatID — ID чата, к которому принадлежит сообщение
	ChatID string `json:"chat_id" db:"chat_id"`

	// SenderID — ID отправителя (User.ID или Contact.ID)
	SenderID string `json:"sender_id" db:"sender_id"`

	// Content — содержимое сообщения (расшифрованное)
	Content string `json:"content" db:"content"`

	// ContentType — тип контента (text, image, file, etc.)
	ContentType string `json:"content_type" db:"content_type"`

	// Status — статус доставки
	Status MessageStatus `json:"status" db:"status"`

	// IsOutgoing — true если сообщение исходящее (от нас)
	IsOutgoing bool `json:"is_outgoing" db:"is_outgoing"`

	// ReplyToID — ID сообщения, на которое это ответ (опционально)
	ReplyToID *string `json:"reply_to_id,omitempty" db:"reply_to_id"`

	// ReplyPreview — информация об исходном сообщении для отображения (не хранится в БД напрямую)
	ReplyPreview *ReplyPreview `json:"reply_preview,omitempty" db:"-"`

	// Timestamp — время создания сообщения (Unix ms)
	Timestamp int64 `json:"timestamp" db:"timestamp"`

	// CreatedAt — локальное время создания записи
	CreatedAt time.Time `json:"created_at" db:"created_at"`

	// UpdatedAt — время последнего обновления (статус и т.д.)
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`

	// Attachments — список вложений
	Attachments []*Attachment `json:"attachments,omitempty" db:"-"`
}

// Attachment представляет вложение (файл/изображение)
type Attachment struct {
	ID           string `json:"id" db:"id"`
	MessageID    string `json:"message_id" db:"message_id"`
	Filename     string `json:"filename" db:"filename"`
	MimeType     string `json:"mime_type" db:"mime_type"`
	Size         int64  `json:"size" db:"size"`
	LocalPath    string `json:"local_path" db:"local_path"`
	IsCompressed bool   `json:"is_compressed" db:"is_compressed"`
	Width        int    `json:"width,omitempty" db:"width"`
	Height       int    `json:"height,omitempty" db:"height"`
}

// Chat представляет чат (диалог) с контактом
type Chat struct {
	// ID — уникальный идентификатор чата
	ID string `json:"id" db:"id"`

	// ContactID — ID контакта в этом чате
	ContactID string `json:"contact_id" db:"contact_id"`

	// LastMessageID — ID последнего сообщения
	LastMessageID *string `json:"last_message_id,omitempty" db:"last_message_id"`

	// UnreadCount — количество непрочитанных сообщений
	UnreadCount int `json:"unread_count" db:"unread_count"`

	// IsMuted — заглушен ли чат
	IsMuted bool `json:"is_muted" db:"is_muted"`

	// CreatedAt — время создания чата
	CreatedAt time.Time `json:"created_at" db:"created_at"`

	// UpdatedAt — время последней активности
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Folder представляет папку с чатами
type Folder struct {
	// ID — уникальный идентификатор папки
	ID string `json:"id" db:"id"`

	// Name — название папки
	Name string `json:"name" db:"name"`

	// Icon — иконка (emoji или URL)
	Icon string `json:"icon" db:"icon"`

	// Position — позиция в списке (для сортировки)
	Position int `json:"position" db:"position"`
}
