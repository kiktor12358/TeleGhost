// Package sqlite реализует SQLite репозиторий для TeleGhost
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"teleghost/internal/core"

	_ "github.com/mattn/go-sqlite3"
)

// Repository — SQLite реализация репозитория
type Repository struct {
	db *sql.DB
}

// New создаёт новый SQLite репозиторий
func New(dbPath string) (*Repository, error) {
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

	repo := &Repository{db: db}

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
		FOREIGN KEY(folder_id) REFERENCES folders(id) ON DELETE CASCADE
	);
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

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.PublicKey, user.PrivateKey, user.Mnemonic,
		user.Nickname, user.Bio, user.Avatar, user.I2PAddress, user.I2PKeys,
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
			status = excluded.status,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}
	msg.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		msg.ID, msg.ChatID, msg.SenderID, msg.Content, msg.ContentType, msg.Status,
		msg.IsOutgoing, msg.ReplyToID, msg.Timestamp, msg.CreatedAt, msg.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
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

	msg := &core.Message{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Content, &msg.ContentType, &msg.Status,
		&msg.IsOutgoing, &msg.ReplyToID, &msg.Timestamp, &msg.CreatedAt, &msg.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	return msg, nil
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

	return messages, rows.Err()
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

	return messages, rows.Err()
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
