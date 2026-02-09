// Package sqlite реализует SQLite репозиторий для TeleGhost
package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"

	_ "github.com/mattn/go-sqlite3"
)

// Repository — SQLite реализация репозитория
type Repository struct {
	db   *sql.DB
	keys *identity.Keys
}

// New создаёт новый SQLite репозиторий
func New(dbPath string, keys *identity.Keys) (*Repository, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Проверяем подключение
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Настройки пула соединений
	db.SetMaxOpenConns(1) // SQLite лучше работает с одним соединением
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	repo := &Repository{
		db:   db,
		keys: keys,
	}

	return repo, nil
}

// Migrate выполняет миграции базы данных
func (r *Repository) Migrate(ctx context.Context) error {
	schema := `
	-- Таблица пользователя (текущий профиль)
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		public_key TEXT NOT NULL UNIQUE,
		private_key BLOB,
		mnemonic TEXT,
		nickname TEXT DEFAULT '',
		bio TEXT DEFAULT '',
		avatar TEXT DEFAULT '',
		i2p_address TEXT DEFAULT '',
		i2p_keys BLOB,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Таблица контактов
	CREATE TABLE IF NOT EXISTS contacts (
		id TEXT PRIMARY KEY,
		public_key TEXT NOT NULL UNIQUE,
		nickname TEXT DEFAULT '',
		bio TEXT DEFAULT '',
		avatar TEXT DEFAULT '',
		i2p_address TEXT NOT NULL,
		chat_id TEXT NOT NULL,
		is_blocked INTEGER DEFAULT 0,
		is_verified INTEGER DEFAULT 0,
		last_seen DATETIME,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Таблица чатов
	CREATE TABLE IF NOT EXISTS chats (
		id TEXT PRIMARY KEY,
		contact_id TEXT NOT NULL UNIQUE,
		last_message_id TEXT,
		unread_count INTEGER DEFAULT 0,
		is_pinned INTEGER DEFAULT 0,
		is_muted INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(contact_id) REFERENCES contacts(id)
	);

	-- Таблица сообщений
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		chat_id TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		content TEXT NOT NULL,
		content_type TEXT DEFAULT 'text',
		status INTEGER DEFAULT 0,
		is_outgoing INTEGER DEFAULT 0,
		reply_to_id TEXT,
		timestamp INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Индексы для быстрого поиска
	CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(chat_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_contacts_public_key ON contacts(public_key);

	-- Таблица папок
	CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		icon TEXT DEFAULT '',
		position INTEGER DEFAULT 0
	);

	-- Таблица связи чатов и папок
	CREATE TABLE IF NOT EXISTS folder_chats (
		folder_id TEXT NOT NULL,
		contact_id TEXT NOT NULL,
		PRIMARY KEY(folder_id, contact_id),
		FOREIGN KEY(folder_id) REFERENCES folders(id) ON DELETE CASCADE,
		FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS message_attachments (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		size INTEGER NOT NULL,
		local_path TEXT NOT NULL,
		is_compressed INTEGER DEFAULT 0,
		width INTEGER DEFAULT 0,
		height INTEGER DEFAULT 0,
		FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_attachments_message_id ON message_attachments(message_id);
	`

	_, err := r.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Close закрывает соединение с БД
func (r *Repository) Close() error {
	return r.db.Close()
}

// === User Methods ===

// GetMyProfile возвращает профиль текущего пользователя
func (r *Repository) GetMyProfile(ctx context.Context) (*core.User, error) {
	query := `
		SELECT id, public_key, private_key, mnemonic, nickname, bio, avatar, 
		       i2p_address, i2p_keys, created_at, updated_at
		FROM users LIMIT 1
	`

	user := &core.User{}
	err := r.db.QueryRowContext(ctx, query).Scan(
		&user.ID, &user.PublicKey, &user.PrivateKey, &user.Mnemonic,
		&user.Nickname, &user.Bio, &user.Avatar, &user.I2PAddress, &user.I2PKeys,
		&user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Профиль ещё не создан
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Дешифруем чувствительные данные
	if r.keys != nil {
		if decryptedPriv, err := r.keys.Decrypt(user.PrivateKey); err == nil {
			user.PrivateKey = decryptedPriv
		}
		if decryptedMnemonic, err := r.keys.Decrypt([]byte(user.Mnemonic)); err == nil {
			user.Mnemonic = string(decryptedMnemonic)
		}
		if len(user.I2PKeys) > 0 {
			if decryptedI2P, err := r.keys.Decrypt(user.I2PKeys); err == nil {
				user.I2PKeys = decryptedI2P
			}
		}
	}

	return user, nil
}

// SaveUser сохраняет или обновляет профиль пользователя
func (r *Repository) SaveUser(ctx context.Context, user *core.User) error {
	query := `
		INSERT INTO users (id, public_key, private_key, mnemonic, nickname, bio, avatar, i2p_address, i2p_keys, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			nickname = excluded.nickname,
			bio = excluded.bio,
			avatar = excluded.avatar,
			i2p_address = excluded.i2p_address,
			i2p_keys = excluded.i2p_keys,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now

	// Шифруем чувствительные данные перед сохранением
	privKey := user.PrivateKey
	mnemonic := []byte(user.Mnemonic)
	i2pKeys := user.I2PKeys

	if r.keys != nil {
		if encPriv, err := r.keys.Encrypt(privKey); err == nil {
			privKey = encPriv
		}
		if encMnemonic, err := r.keys.Encrypt(mnemonic); err == nil {
			mnemonic = encMnemonic
		}
		if len(i2pKeys) > 0 {
			if encI2P, err := r.keys.Encrypt(i2pKeys); err == nil {
				i2pKeys = encI2P
			}
		}
	}

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.PublicKey, privKey, mnemonic,
		user.Nickname, user.Bio, user.Avatar, user.I2PAddress, i2pKeys,
		user.CreatedAt, user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	return nil
}

// UpdateMyProfile обновляет профиль пользователя
func (r *Repository) UpdateMyProfile(ctx context.Context, nickname, bio, avatar string) error {
	query := `
		UPDATE users SET nickname = ?, bio = ?, avatar = ?, updated_at = ?
		WHERE id = (SELECT id FROM users LIMIT 1)
	`

	_, err := r.db.ExecContext(ctx, query, nickname, bio, avatar, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	return nil
}

// === Contact Methods ===

// SaveContact сохраняет контакт
func (r *Repository) SaveContact(ctx context.Context, contact *core.Contact) error {
	query := `
		INSERT INTO contacts (id, public_key, nickname, bio, avatar, i2p_address, chat_id, 
		                      is_blocked, is_verified, last_seen, added_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			nickname = excluded.nickname,
			bio = excluded.bio,
			avatar = excluded.avatar,
			i2p_address = excluded.i2p_address,
			is_blocked = excluded.is_blocked,
			is_verified = excluded.is_verified,
			last_seen = excluded.last_seen,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	if contact.AddedAt.IsZero() {
		contact.AddedAt = now
	}
	contact.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		contact.ID, contact.PublicKey, contact.Nickname, contact.Bio, contact.Avatar,
		contact.I2PAddress, contact.ChatID, contact.IsBlocked, contact.IsVerified,
		contact.LastSeen, contact.AddedAt, contact.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save contact: %w", err)
	}

	return nil
}

// GetContact возвращает контакт по ID
func (r *Repository) GetContact(ctx context.Context, id string) (*core.Contact, error) {
	query := `
		SELECT id, public_key, nickname, bio, avatar, i2p_address, chat_id,
		       is_blocked, is_verified, last_seen, added_at, updated_at
		FROM contacts WHERE id = ?
	`

	contact := &core.Contact{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&contact.ID, &contact.PublicKey, &contact.Nickname, &contact.Bio, &contact.Avatar,
		&contact.I2PAddress, &contact.ChatID, &contact.IsBlocked, &contact.IsVerified,
		&contact.LastSeen, &contact.AddedAt, &contact.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}

	return contact, nil
}

// GetContactByPublicKey возвращает контакт по его публичному ключу
func (r *Repository) GetContactByPublicKey(ctx context.Context, publicKey string) (*core.Contact, error) {
	query := `
		SELECT id, public_key, nickname, bio, avatar, i2p_address, chat_id,
		       is_blocked, is_verified, last_seen, added_at, updated_at
		FROM contacts WHERE public_key = ?
	`

	contact := &core.Contact{}
	err := r.db.QueryRowContext(ctx, query, publicKey).Scan(
		&contact.ID, &contact.PublicKey, &contact.Nickname, &contact.Bio, &contact.Avatar,
		&contact.I2PAddress, &contact.ChatID, &contact.IsBlocked, &contact.IsVerified,
		&contact.LastSeen, &contact.AddedAt, &contact.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact by pubkey: %w", err)
	}

	return contact, nil
}

// GetContactByAddress возвращает контакт по его I2P адресу
func (r *Repository) GetContactByAddress(ctx context.Context, address string) (*core.Contact, error) {
	query := `
		SELECT id, public_key, nickname, bio, avatar, i2p_address, chat_id,
		       is_blocked, is_verified, last_seen, added_at, updated_at
		FROM contacts WHERE i2p_address = ?
	`

	contact := &core.Contact{}
	err := r.db.QueryRowContext(ctx, query, address).Scan(
		&contact.ID, &contact.PublicKey, &contact.Nickname, &contact.Bio, &contact.Avatar,
		&contact.I2PAddress, &contact.ChatID, &contact.IsBlocked, &contact.IsVerified,
		&contact.LastSeen, &contact.AddedAt, &contact.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact by address: %w", err)
	}

	return contact, nil
}

// ListContacts возвращает список всех контактов
func (r *Repository) ListContacts(ctx context.Context) ([]*core.Contact, error) {
	query := `
		SELECT id, public_key, nickname, bio, avatar, i2p_address, chat_id,
		       is_blocked, is_verified, last_seen, added_at, updated_at
		FROM contacts
		ORDER BY nickname ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}
	defer rows.Close()

	var contacts []*core.Contact
	for rows.Next() {
		contact := &core.Contact{}
		err := rows.Scan(
			&contact.ID, &contact.PublicKey, &contact.Nickname, &contact.Bio, &contact.Avatar,
			&contact.I2PAddress, &contact.ChatID, &contact.IsBlocked, &contact.IsVerified,
			&contact.LastSeen, &contact.AddedAt, &contact.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contact: %w", err)
		}
		contacts = append(contacts, contact)
	}

	return contacts, rows.Err()
}

// DeleteContact удаляет контакт по ID
func (r *Repository) DeleteContact(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM contacts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}
	return nil
}

// === Message Methods ===

// SaveMessage сохраняет сообщение
func (r *Repository) SaveMessage(ctx context.Context, msg *core.Message) error {
	query := `
		INSERT INTO messages (id, chat_id, sender_id, content, content_type, status, 
		                      is_outgoing, reply_to_id, timestamp, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			content_type = excluded.content_type,
			status = excluded.status,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}
	msg.UpdatedAt = now

	// Шифруем контент сообщения
	content := msg.Content
	if r.keys != nil && content != "" {
		if encContent, err := r.keys.Encrypt([]byte(content)); err == nil {
			content = base64.StdEncoding.EncodeToString(encContent)
		}
	}

	_, err := r.db.ExecContext(ctx, query,
		msg.ID, msg.ChatID, msg.SenderID, content, msg.ContentType, msg.Status,
		msg.IsOutgoing, msg.ReplyToID, msg.Timestamp, msg.CreatedAt, msg.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	// Сохраняем вложения
	// Сначала удаляем старые, чтобы обновить список (например, при переходе от Offer к Real файлам)
	_, err = r.db.ExecContext(ctx, "DELETE FROM message_attachments WHERE message_id = ?", msg.ID)
	if err != nil {
		return fmt.Errorf("failed to delete old attachments: %w", err)
	}

	if len(msg.Attachments) > 0 {
		attQuery := `INSERT INTO message_attachments (id, message_id, filename, mime_type, size, local_path, is_compressed, width, height) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		for _, att := range msg.Attachments {
			// Ensure MessageID is set
			if att.MessageID == "" {
				att.MessageID = msg.ID
			}

			localPath := att.LocalPath
			if r.keys != nil && localPath != "" {
				if encPath, err := r.keys.Encrypt([]byte(localPath)); err == nil {
					localPath = base64.StdEncoding.EncodeToString(encPath)
				}
			}

			_, err := r.db.ExecContext(ctx, attQuery, att.ID, att.MessageID, att.Filename, att.MimeType, att.Size, localPath, att.IsCompressed, att.Width, att.Height)
			if err != nil {
				return fmt.Errorf("failed to save attachment %s: %w", att.ID, err)
			}
		}
	}

	return nil
}

// GetMessage возвращает сообщение по ID
func (r *Repository) GetMessage(ctx context.Context, id string) (*core.Message, error) {
	query := `
		SELECT id, chat_id, sender_id, content, content_type, status,
		       is_outgoing, reply_to_id, timestamp, created_at, updated_at
		FROM messages WHERE id = ?
	`

	msg, err := r.scanMessage(r.db.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := r.enrichMessagesWithAttachments(ctx, []*core.Message{msg}); err != nil {
		return nil, fmt.Errorf("failed to enrich message: %w", err)
	}

	return msg, nil
}

func (r *Repository) scanMessage(row interface {
	Scan(dest ...interface{}) error
}) (*core.Message, error) {
	msg := &core.Message{}
	err := row.Scan(
		&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Content, &msg.ContentType, &msg.Status,
		&msg.IsOutgoing, &msg.ReplyToID, &msg.Timestamp, &msg.CreatedAt, &msg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Дешифруем контент
	if r.keys != nil && msg.Content != "" {
		if decoded, err := base64.StdEncoding.DecodeString(msg.Content); err == nil {
			if decrypted, err := r.keys.Decrypt(decoded); err == nil {
				msg.Content = string(decrypted)
			}
		}
	}

	return msg, nil
}

// enrichMessagesWithAttachments загружает вложения для списка сообщений
func (r *Repository) enrichMessagesWithAttachments(ctx context.Context, messages []*core.Message) error {
	if len(messages) == 0 {
		return nil
	}

	msgMap := make(map[string]*core.Message)
	ids := make([]interface{}, len(messages))
	for i, m := range messages {
		msgMap[m.ID] = m
		ids[i] = m.ID
		m.Attachments = make([]*core.Attachment, 0)
	}

	// SQLite limit for parameters is usually 999 or higher, but safer to batch if needed.
	// For now simple IN clause.
	placeholders := ""
	for i := 0; i < len(ids); i++ {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}

	query := fmt.Sprintf(`SELECT id, message_id, filename, mime_type, size, local_path, is_compressed, width, height FROM message_attachments WHERE message_id IN (%s)`, placeholders)

	rows, err := r.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		att := &core.Attachment{}
		var width, height sql.NullInt32 // Handle potentially null if old records (though declared default 0)
		err := rows.Scan(&att.ID, &att.MessageID, &att.Filename, &att.MimeType, &att.Size, &att.LocalPath, &att.IsCompressed, &width, &height)
		if err != nil {
			return err
		}
		att.Width = int(width.Int32)
		att.Height = int(height.Int32)

		// Дешифруем путь к файлу
		if r.keys != nil && att.LocalPath != "" {
			if decoded, err := base64.StdEncoding.DecodeString(att.LocalPath); err == nil {
				if decrypted, err := r.keys.Decrypt(decoded); err == nil {
					att.LocalPath = string(decrypted)
				}
			}
		}

		if msg, ok := msgMap[att.MessageID]; ok {
			msg.Attachments = append(msg.Attachments, att)
		}
	}
	return rows.Err()
}

// GetChatHistory возвращает историю сообщений чата с пагинацией
func (r *Repository) GetChatHistory(ctx context.Context, chatID string, limit, offset int) ([]*core.Message, error) {
	query := `
		SELECT id, chat_id, sender_id, content, content_type, status,
		       is_outgoing, reply_to_id, timestamp, created_at, updated_at
		FROM messages
		WHERE chat_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, chatID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat history: %w", err)
	}
	defer rows.Close()

	var messages []*core.Message
	for rows.Next() {
		msg := &core.Message{}
		err := rows.Scan(
			&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Content, &msg.ContentType, &msg.Status,
			&msg.IsOutgoing, &msg.ReplyToID, &msg.Timestamp, &msg.CreatedAt, &msg.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	return messages, r.enrichMessagesWithAttachments(ctx, messages)
}

// UpdateMessageStatus обновляет статус доставки сообщения
func (r *Repository) UpdateMessageStatus(ctx context.Context, id string, status core.MessageStatus) error {
	query := `UPDATE messages SET status = ?, updated_at = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	return nil
}

// DeleteMessage удаляет сообщение по ID
func (r *Repository) DeleteMessage(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM messages WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	return nil
}

// UpdateMessageContent обновляет содержимое сообщения (редактирование)
func (r *Repository) UpdateMessageContent(ctx context.Context, id, newContent string) error {
	query := `UPDATE messages SET content = ?, updated_at = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, newContent, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update message content: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", id)
	}

	return nil
}

// SearchMessages ищет сообщения по тексту в чате
func (r *Repository) SearchMessages(ctx context.Context, chatID, queryStr string) ([]*core.Message, error) {
	query := `
		SELECT id, chat_id, sender_id, content, content_type, status,
		       is_outgoing, reply_to_id, timestamp, created_at, updated_at
		FROM messages
		WHERE chat_id = ? AND content LIKE ?
		ORDER BY timestamp DESC
		LIMIT 50
	`

	rows, err := r.db.QueryContext(ctx, query, chatID, "%"+queryStr+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	var messages []*core.Message
	for rows.Next() {
		msg := &core.Message{}
		err := rows.Scan(
			&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Content, &msg.ContentType, &msg.Status,
			&msg.IsOutgoing, &msg.ReplyToID, &msg.Timestamp, &msg.CreatedAt, &msg.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	return messages, r.enrichMessagesWithAttachments(ctx, messages)
}

// === Chat Methods ===

// GetChat возвращает чат по ID
func (r *Repository) GetChat(ctx context.Context, id string) (*core.Chat, error) {
	query := `SELECT id, contact_id, last_message_id, unread_count, is_muted, created_at, updated_at FROM chats WHERE id = ?`

	chat := &core.Chat{}
	var lastMsgID sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&chat.ID, &chat.ContactID, &lastMsgID, &chat.UnreadCount, &chat.IsMuted, &chat.CreatedAt, &chat.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("chat not found")
	}
	if err != nil {
		return nil, err
	}

	if lastMsgID.Valid {
		chat.LastMessageID = &lastMsgID.String
	}

	return chat, nil
}

// === Folder Methods ===

// CreateFolder создаёт новую папку
func (r *Repository) CreateFolder(ctx context.Context, folder *core.Folder) error {
	query := `INSERT INTO folders (id, name, icon, position) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, icon=excluded.icon, position=excluded.position`
	_, err := r.db.ExecContext(ctx, query, folder.ID, folder.Name, folder.Icon, folder.Position)
	return err
}

// GetFolders возвращает все папки
func (r *Repository) GetFolders(ctx context.Context) ([]*core.Folder, error) {
	query := `SELECT id, name, icon, position FROM folders ORDER BY position ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []*core.Folder
	for rows.Next() {
		f := &core.Folder{}
		if err := rows.Scan(&f.ID, &f.Name, &f.Icon, &f.Position); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// DeleteFolder удаляет папку
func (r *Repository) DeleteFolder(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM folders WHERE id = ?", id)
	return err
}

// AddChatToFolder добавляет чат (контакт) в папку
func (r *Repository) AddChatToFolder(ctx context.Context, folderID, contactID string) error {
	_, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO folder_chats (folder_id, contact_id) VALUES (?, ?)", folderID, contactID)
	return err
}

// RemoveChatFromFolder удаляет чат из папки
func (r *Repository) RemoveChatFromFolder(ctx context.Context, folderID, contactID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM folder_chats WHERE folder_id = ? AND contact_id = ?", folderID, contactID)
	return err
}

// GetFolderChats возвращает список ID контактов в папке
func (r *Repository) GetFolderChats(ctx context.Context, folderID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT contact_id FROM folder_chats WHERE folder_id = ?", folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chatIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		chatIDs = append(chatIDs, id)
	}
	return chatIDs, nil
}
