// Package messenger реализует сервис обмена сообщениями через I2P
package messenger

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"time"

	"teleghost/internal/core"
	"teleghost/internal/core/identity"
	"teleghost/internal/network/router"
	pb "teleghost/internal/proto"

	"google.golang.org/protobuf/proto"
)

const (
	// ProtocolVersion текущая версия протокола
	ProtocolVersion = 1

	// MaxPacketSize максимальный размер пакета (10MB for images)
	MaxPacketSize = 50 * 1024 * 1024

	// HeartbeatInterval интервал отправки heartbeat
	HeartbeatInterval = 60 * time.Second

	// ConnectionTimeout таймаут соединения (для I2P нужны большие значения)
	ConnectionTimeout = 3 * time.Minute

	// ReadTimeout таймаут чтения (I2P медленный, особенно при первом подключении)
	ReadTimeout = 5 * time.Minute
)

// FileOfferHandler обработчик входящих предложений файла
type FileOfferHandler func(senderPubKey, messageID, chatID string, filenames []string, totalSize int64, fileCount int32)

// FileResponseHandler обработчик ответов на предложение файла
type FileResponseHandler func(senderPubKey, messageID, chatID string, accepted bool)

// MessageHandler обработчик входящих сообщений
type MessageHandler func(msg *core.Message, senderPubKey, senderAddr string)

// ContactRequestHandler обработчик запросов дружбы
type ContactRequestHandler func(senderPubKey, nickname, i2pAddress string)

// ProfileUpdateHandler обработчик обновлений профиля
type ProfileUpdateHandler func(senderPubKey, nickname, bio string, avatar []byte, senderAddr string)

// Service — мессенджер сервис
type Service struct {
	router         *router.SAMRouter
	identity       *identity.Keys
	handler        MessageHandler
	contactHandler ContactRequestHandler

	profileHandler        ProfileUpdateHandler
	profileRequestHandler ProfileRequestHandler
	fileOfferHandler      FileOfferHandler
	fileResponseHandler   FileResponseHandler

	attachmentSaver AttachmentSaver
	connections     map[string]net.Conn // destination -> connection
	connMu          sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	started         bool
	mu              sync.Mutex
	myNickname      string // Для отправки в handshake
}

// NewService создаёт новый MessengerService
func NewService(r *router.SAMRouter, id *identity.Keys, handler MessageHandler) *Service {
	return &Service{
		router:      r,
		identity:    id,
		handler:     handler,
		connections: make(map[string]net.Conn),
		myNickname:  "User", // Default
	}
}

// SetContactHandler устанавливает обработчик запросов дружбы
func (s *Service) SetContactHandler(h ContactRequestHandler) {
	s.contactHandler = h
}

// SetNickname устанавливает никнейм для handshake
func (s *Service) SetNickname(nickname string) {
	s.myNickname = nickname
}

// Start запускает сервис: listener и heartbeat
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("service already started")
	}
	s.started = true
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// Запускаем listener в горутине
	s.wg.Add(1)
	go s.listenLoop()

	// Запускаем heartbeat в горутине
	s.wg.Add(1)
	go s.heartbeatLoop()

	dest := s.router.GetDestination()
	showLen := min(32, len(dest))
	log.Printf("[Messenger] Started. My destination: %s...", dest[:showLen])

	return nil
}

// Stop останавливает сервис
func (s *Service) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = false
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	// Закрываем все соединения
	s.connMu.Lock()
	for dest, conn := range s.connections {
		_ = conn.Close()
		delete(s.connections, dest)
	}
	s.connMu.Unlock()

	// Ждём завершения горутин
	s.wg.Wait()

	log.Printf("[Messenger] Stopped")
	return nil
}

// GetDestination возвращает наш I2P Destination
func (s *Service) GetDestination() string {
	return s.router.GetDestination()
}

// SendMessage отправляет сообщение получателю
func (s *Service) SendMessage(destination string, packet *pb.Packet) error {
	// Устанавливаем версию и подписываем
	packet.Version = ProtocolVersion
	packet.SenderPubKey = []byte(s.identity.PublicKeyBase64)

	// Подписываем payload
	if len(packet.Payload) > 0 {
		packet.Signature = s.identity.SignMessage(packet.Payload)
	}

	// Получаем или создаём соединение
	showDest := destination[:min(16, len(destination))]
	log.Printf("[Messenger] Getting connection for %s...", showDest)
	conn, err := s.getOrCreateConnection(destination)
	if err != nil {
		log.Printf("[Messenger] ERROR: connection failed for %s: %v", showDest, err)
		return fmt.Errorf("connection failed: %w", err)
	}

	// Сериализуем пакет
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	log.Printf("[Messenger] Sending packet type %v (%d bytes) to %s...", packet.Type, len(data), showDest)
	// Отправляем: 4 байта размер + данные
	if err := s.writePacket(conn, data); err != nil {
		log.Printf("[Messenger] ERROR: write failed for %s: %v", showDest, err)
		// При ошибке удаляем соединение из пула
		s.removeConnection(destination)
		return fmt.Errorf("send failed: %w", err)
	}

	log.Printf("[Messenger] Packet sent successfully to %s", showDest)
	return nil
}

// SendTextMessageWithID создаёт и отправляет текстовое сообщение с указанным ID
func (s *Service) SendTextMessageWithID(destination, chatID, messageID, content, replyToID string) error {
	now := time.Now().UnixMilli()

	// Создаём TextMessage
	textMsg := &pb.TextMessage{
		ChatId:    chatID,
		Content:   content,
		Timestamp: now,
		MessageId: messageID,
		ReplyToId: replyToID,
	}

	payload, err := proto.Marshal(textMsg)
	if err != nil {
		return fmt.Errorf("marshal text message failed: %w", err)
	}

	packet := &pb.Packet{
		Type:    pb.PacketType_TEXT_MESSAGE,
		Payload: payload,
	}

	return s.SendMessage(destination, packet)
}

// SendFileOffer отправляет предложение передачи файлов
func (s *Service) SendFileOffer(destination, chatID, messageID string, filenames []string, totalSize int64, fileCount int32) error {
	offer := &pb.FileOffer{
		MessageId: messageID,
		ChatId:    chatID,
		Filenames: filenames,
		TotalSize: totalSize,
		FileCount: fileCount,
	}

	payload, err := proto.Marshal(offer)
	if err != nil {
		return fmt.Errorf("marshal file offer failed: %w", err)
	}

	packet := &pb.Packet{
		Type:    pb.PacketType_FILE_OFFER,
		Payload: payload,
	}

	log.Printf("[Messenger] Sending file offer (id=%s) to %s...", messageID[:min(8, len(messageID))], destination[:min(32, len(destination))])
	return s.SendMessage(destination, packet)
}

// SendFileResponse отправляет ответ на предложение
func (s *Service) SendFileResponse(destination, chatID, messageID string, accepted bool) error {
	resp := &pb.FileResponse{
		MessageId: messageID,
		ChatId:    chatID,
		Accepted:  accepted,
	}

	payload, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal file response failed: %w", err)
	}

	packet := &pb.Packet{
		Type:    pb.PacketType_FILE_RESPONSE,
		Payload: payload,
	}

	log.Printf("[Messenger] Sending file response (accepted=%v) to %s...", accepted, destination[:min(32, len(destination))])
	return s.SendMessage(destination, packet)
}

// SendHeartbeat отправляет heartbeat пакет
func (s *Service) SendHeartbeat(destination string) error {
	packet := &pb.Packet{
		Type:    pb.PacketType_HEARTBEAT,
		Payload: []byte{},
	}

	return s.SendMessage(destination, packet)
}

// SendHandshake отправляет handshake пакет для установления контакта
func (s *Service) SendHandshake(destination string) error {
	now := time.Now().UnixMilli()

	handshake := &pb.Handshake{
		InitiatorPubKey: []byte(s.identity.PublicKeyBase64),
		Timestamp:       now,
		Nickname:        s.myNickname,
		I2PAddress:      s.router.GetDestination(),
	}

	payload, err := proto.Marshal(handshake)
	if err != nil {
		return fmt.Errorf("marshal handshake failed: %w", err)
	}

	packet := &pb.Packet{
		Type:    pb.PacketType_HANDSHAKE,
		Payload: payload,
	}

	log.Printf("[Messenger] Sending handshake to %s...", destination[:min(32, len(destination))])
	return s.SendMessage(destination, packet)
}

// SendProfileRequest отправляет запрос на обновление профиля
func (s *Service) SendProfileRequest(destination string) error {
	packet := &pb.Packet{
		Type:    pb.PacketType_PROFILE_REQUEST,
		Payload: []byte{}, // Empty payload
	}

	log.Printf("[Messenger] Sending profile request to %s...", destination[:min(32, len(destination))])
	return s.SendMessage(destination, packet)
}

// SendProfileUpdate отправляет обновление нашего профиля получателю
func (s *Service) SendProfileUpdate(destination, nickname, bio string, avatar []byte) error {
	update := &pb.ProfileUpdate{
		Nickname: nickname,
		Bio:      bio,
		Avatar:   avatar,
	}

	payload, err := proto.Marshal(update)
	if err != nil {
		return fmt.Errorf("marshal profile update failed: %w", err)
	}

	packet := &pb.Packet{
		Type:    pb.PacketType_PROFILE_UPDATE,
		Payload: payload,
	}

	log.Printf("[Messenger] Sending profile update to %s...", destination[:min(32, len(destination))])
	return s.SendMessage(destination, packet)
}

// getOrCreateConnection получает существующее или создаёт новое соединение
func (s *Service) getOrCreateConnection(destination string) (net.Conn, error) {
	s.connMu.RLock()
	conn, exists := s.connections[destination]
	s.connMu.RUnlock()

	if exists {
		return conn, nil
	}

	// Dial с таймаутом БЕЗ блокировки всего пула
	log.Printf("[Messenger] Dialing %s...", destination[:min(16, len(destination))])
	newConn, err := s.router.Dial(destination)
	if err != nil {
		return nil, err
	}

	// Сохраняем новое соединение
	s.connMu.Lock()
	defer s.connMu.Unlock()

	// На случай, если кто-то другой уже успел создать соединение
	if conn, exists = s.connections[destination]; exists {
		_ = newConn.Close()
		return conn, nil
	}

	s.connections[destination] = newConn
	log.Printf("[Messenger] Connected to %s...", destination[:min(16, len(destination))])

	return newConn, nil
}

// removeConnection удаляет соединение из пула
func (s *Service) removeConnection(destination string) {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if conn, exists := s.connections[destination]; exists {
		_ = conn.Close()
		delete(s.connections, destination)
	}
}

// writePacket пишет пакет в соединение (length-prefixed)
func (s *Service) writePacket(conn net.Conn, data []byte) error {
	// Устанавливаем deadline
	_ = conn.SetWriteDeadline(time.Now().Add(ConnectionTimeout))

	// Пишем размер (4 байта, big endian)
	const maxUint32 = math.MaxUint32
	if uint64(len(data)) > maxUint32 {
		return fmt.Errorf("packet too large for uint32: %d", len(data))
	}
	if len(data) > 100*1024*1024 { // 100 MB limit for safety
		return fmt.Errorf("packet too large: %d", len(data))
	}
	sizeBuf := make([]byte, 4)
	// #nosec G115
	binary.BigEndian.PutUint32(sizeBuf, uint32(len(data)))

	if _, err := conn.Write(sizeBuf); err != nil {
		return err
	}

	// Пишем данные
	if _, err := conn.Write(data); err != nil {
		return err
	}

	return nil
}

// readPacket читает пакет из соединения
func (s *Service) readPacket(conn net.Conn) ([]byte, error) {
	// Устанавливаем deadline
	_ = conn.SetReadDeadline(time.Now().Add(ReadTimeout))

	// Читаем размер
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, sizeBuf); err != nil {
		return nil, err
	}

	size := binary.BigEndian.Uint32(sizeBuf)
	if size > MaxPacketSize {
		return nil, fmt.Errorf("packet too large: %d", size)
	}

	// Читаем данные
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}

	return data, nil
}

// listenLoop принимает входящие соединения
func (s *Service) listenLoop() {
	defer s.wg.Done()

	listener, err := s.router.Listen()
	if err != nil {
		log.Printf("[Messenger] Failed to get listener: %v", err)
		return
	}

	log.Printf("[Messenger] Listening for incoming connections...")

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// SAM StreamListener не поддерживает deadline напрямую,
		// используем Accept() в цикле с проверкой context

		conn, err := listener.Accept()
		if err != nil {
			// Если роутер остановлен, выходим
			if !s.router.IsReady() {
				return
			}

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if s.ctx.Err() != nil {
				return
			}
			// Проверяем на ошибку закрытия (строковое сравнение для надежности с разными реализациями)
			if err.Error() == "use of closed network connection" || err.Error() == "listener closed" {
				return
			}

			log.Printf("[Messenger] Accept error: %v", err)
			continue
		}

		// Обрабатываем соединение в отдельной горутине
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection обрабатывает входящее соединение
func (s *Service) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	if i2pAddr, ok := conn.RemoteAddr().(interface{ Base64() string }); ok {
		remoteAddr = i2pAddr.Base64()
	}
	log.Printf("[Messenger] Incoming connection from %s...", remoteAddr[:min(32, len(remoteAddr))])

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Читаем пакет
		data, err := s.readPacket(conn)
		if err != nil {
			if err == io.EOF || s.ctx.Err() != nil {
				return
			}
			log.Printf("[Messenger] Read error: %v", err)
			return
		}

		// Десериализуем
		packet := &pb.Packet{}
		if err := proto.Unmarshal(data, packet); err != nil {
			log.Printf("[Messenger] Unmarshal error: %v", err)
			continue
		}

		// Обрабатываем пакет
		s.handlePacket(packet, remoteAddr)
	}
}

// handlePacket обрабатывает входящий пакет
func (s *Service) handlePacket(packet *pb.Packet, remoteAddr string) {
	senderPubKey := string(packet.SenderPubKey)

	// Проверяем подпись (если есть payload)
	if len(packet.Payload) > 0 && len(packet.Signature) > 0 {
		valid, err := identity.VerifySignatureBase64(senderPubKey, packet.Payload, packet.Signature)
		if err != nil || !valid {
			log.Printf("[Messenger] Invalid signature from %s...", senderPubKey[:16])
			return
		}
	}

	switch packet.Type {
	case pb.PacketType_HEARTBEAT:
		log.Printf("[Messenger] Heartbeat from %s...", senderPubKey[:16])

	case pb.PacketType_TEXT_MESSAGE:
		s.handleTextMessage(packet, senderPubKey, remoteAddr)

	case pb.PacketType_PROFILE_UPDATE:
		s.handleProfileUpdate(packet, senderPubKey, remoteAddr)

	case pb.PacketType_HANDSHAKE:
		s.handleHandshake(packet, senderPubKey)

	case pb.PacketType_PROFILE_REQUEST:
		s.handleProfileRequest(packet, senderPubKey)

	case pb.PacketType_FILE_OFFER:
		s.handleFileOffer(packet, senderPubKey)

	case pb.PacketType_FILE_RESPONSE:
		s.handleFileResponse(packet, senderPubKey)

	default:
		log.Printf("[Messenger] Unknown packet type: %v", packet.Type)
	}
}

// handleProfileRequest обрабатывает запрос на обновление профиля
func (s *Service) handleProfileRequest(_ *pb.Packet, senderPubKey string) {
	log.Printf("[Messenger] Profile request from %s", senderPubKey[:min(16, len(senderPubKey))])

	// Получаем текущий профиль (или используем дефолтные значения из s.myNickname)
	// В идеале сервис должен иметь доступ к репозиторию, но сейчас у нас есть s.profileHandler
	// Который работает ВХОДЯЩИЕ обновления.
	// Нам нужно отправить ИСХОДЯЩЕЕ обновление.
	// Для простоты, мы можем добавить Callback "GetMyProfile" или передавать данные при старте.
	// Но пока используем s.myNickname, а био и аватар пустые (или нужно прокинуть их).
	// TODO: Прокинуть полный профиль в MessengerService

	// Отправляем ответ с текущим никнеймом пакетом PROFILE_UPDATE
	// Аватар и био пока пустые, если нет доступа к ним
	// В App.go мы можем отправить Update сразу после получения запроса, но это сложно связать.
	// Поэтому лучше Messenger просто уведомляет App "запросили профиль", а App отправляет Update.
	if s.profileRequestHandler != nil {
		s.profileRequestHandler(senderPubKey)
	}
}

// ProfileRequestHandler handler for incoming profile requests
type ProfileRequestHandler func(requestorPubKey string)

// AttachmentSaver сохраняет вложение и возвращает путь
type AttachmentSaver func(filename string, data []byte) (string, error)

// SetAttachmentSaver устанавливает функцию сохранения вложений
func (s *Service) SetAttachmentSaver(saver AttachmentSaver) {
	s.attachmentSaver = saver
}

// SendAttachmentMessage отправляет сообщение с вложениями
func (s *Service) SendAttachmentMessage(destination, chatID, content string, attachments []*pb.Attachment) error {
	now := time.Now().UnixMilli()

	// Создаём TextMessage (MessageId is generated here but ignored by App.go for sent messages which creates its own ID)
	// Wait, app.go uses its own ID for local storage, but sends this ID.
	// If we use file offer, we need to match IDs.
	// For normal flow, app.go generates ID before sending? No, app.go calls SendAttachmentMessage and saves whatever.
	// BUT with FileOffer, we generate ID FIRST, then send Offer, then send File.
	// So we need to allow passing MessageID externally if needed.
	// Or we can assume app.go handles consistency.
	// For now, let's keep it simple. If we need explicit ID, we should add it to arguments.
	// But `SendAttachmentMessage` generates new ID here: `fmt.Sprintf("%d-%s", now, s.identity.UserID[:8])`.
	// This is problematic for File Flow where ID is predetermined.
	// I should overload this or start passing MessageID.
	// I'll add `messageID` optional parameter? No, Go doesn't support optional.
	// I'll create `SendAttachmentMessageWithID`.

	msgID := fmt.Sprintf("%d-%s", now, s.identity.UserID[:8])
	return s.SendAttachmentMessageWithID(destination, chatID, msgID, content, "", attachments)
}

// SendAttachmentMessageWithID отправляет сообщение с вложениями и указанным ID
func (s *Service) SendAttachmentMessageWithID(destination, chatID, messageID, content, replyToID string, attachments []*pb.Attachment) error {
	now := time.Now().UnixMilli()

	// Создаём TextMessage
	textMsg := &pb.TextMessage{
		ChatId:      chatID,
		Content:     content,
		Timestamp:   now,
		MessageId:   messageID,
		Attachments: attachments,
		ReplyToId:   replyToID,
	}

	payload, err := proto.Marshal(textMsg)
	if err != nil {
		return fmt.Errorf("marshal text message failed: %w", err)
	}

	packet := &pb.Packet{
		Version:      ProtocolVersion,
		Type:         pb.PacketType_TEXT_MESSAGE,
		SenderPubKey: []byte(s.identity.PublicKeyBase64),
		Payload:      payload,
	}

	// Подписываем payload
	if len(packet.Payload) > 0 {
		packet.Signature = s.identity.SignMessage(packet.Payload)
	}

	// Получаем или создаём соединение
	conn, err := s.getOrCreateConnection(destination)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Сериализуем пакет
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("marshal packet failed: %w", err)
	}

	if len(data) > MaxPacketSize {
		return fmt.Errorf("packet too large: %d > %d", len(data), MaxPacketSize)
	}

	// Отправляем
	if err := s.writePacket(conn, data); err != nil {
		s.removeConnection(destination)
		return fmt.Errorf("send failed: %w", err)
	}

	return nil
}

// handleTextMessage обрабатывает текстовое сообщение
func (s *Service) handleTextMessage(packet *pb.Packet, senderPubKey, remoteAddr string) {
	textMsg := &pb.TextMessage{}
	if err := proto.Unmarshal(packet.Payload, textMsg); err != nil {
		log.Printf("[Messenger] Failed to unmarshal TextMessage: %v", err)
		return
	}

	// Message Length Limit Check
	if len(textMsg.Content) > 4096 {
		log.Printf("[Messenger] Rejected message from %s: content too long (%d > 4096)", senderPubKey[:min(16, len(senderPubKey))], len(textMsg.Content))
		return
	}

	// Создаём core.Message для сохранения
	msg := &core.Message{
		ID:          textMsg.MessageId,
		ChatID:      textMsg.ChatId,
		SenderID:    senderPubKey,
		Content:     textMsg.Content,
		ContentType: "text",
		Status:      core.MessageStatusDelivered,
		IsOutgoing:  false,
		Timestamp:   textMsg.Timestamp,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if textMsg.ReplyToId != "" {
		msg.ReplyToID = &textMsg.ReplyToId
	}

	// Обрабатываем вложения
	if len(textMsg.Attachments) > 0 {
		msg.ContentType = "mixed" // или оставить text, но с вложениями
		for _, att := range textMsg.Attachments {
			path := ""
			// Сохраняем файл если есть saver
			if s.attachmentSaver != nil {
				savedPath, err := s.attachmentSaver(att.Filename, att.Data)
				if err != nil {
					log.Printf("[Messenger] Failed to save attachment %s: %v", att.Filename, err)
				} else {
					path = savedPath
				}
			}

			coreAtt := &core.Attachment{
				ID:           att.Id,
				MessageID:    msg.ID,
				Filename:     att.Filename,
				MimeType:     att.MimeType,
				Size:         att.Size,
				LocalPath:    path,
				IsCompressed: att.IsCompressed,
				Width:        int(att.Width),
				Height:       int(att.Height),
			}
			msg.Attachments = append(msg.Attachments, coreAtt)
		}
	}

	log.Printf("[Messenger] Received message: %s... (atts: %d)", textMsg.Content[:min(20, len(textMsg.Content))], len(textMsg.Attachments))

	// Вызываем callback для обработки
	if s.handler != nil {
		s.handler(msg, senderPubKey, remoteAddr)
	}
}

// handleProfileUpdate обрабатывает обновление профиля
func (s *Service) handleProfileUpdate(packet *pb.Packet, senderPubKey, senderAddr string) {
	profileUpdate := &pb.ProfileUpdate{}
	if err := proto.Unmarshal(packet.Payload, profileUpdate); err != nil {
		log.Printf("[Messenger] Failed to unmarshal ProfileUpdate: %v", err)
		return
	}

	log.Printf("[Messenger] Profile update from %s: %s", senderPubKey[:min(16, len(senderPubKey))], profileUpdate.Nickname)
	if s.profileHandler != nil {
		s.profileHandler(senderPubKey, profileUpdate.Nickname, profileUpdate.Bio, profileUpdate.Avatar, senderAddr)
	}
}

// handleHandshake обрабатывает рукопожатие
func (s *Service) handleHandshake(packet *pb.Packet, senderPubKey string) {
	handshake := &pb.Handshake{}
	if err := proto.Unmarshal(packet.Payload, handshake); err != nil {
		log.Printf("[Messenger] Failed to unmarshal Handshake: %v", err)
		return
	}

	nickname := handshake.Nickname
	if nickname == "" {
		nickname = "Unknown"
	}

	i2pAddress := handshake.I2PAddress

	log.Printf("[Messenger] Handshake from %s (nickname: %s)", senderPubKey[:min(16, len(senderPubKey))], nickname)

	// Вызываем callback для создания контакта
	if s.contactHandler != nil {
		s.contactHandler(senderPubKey, nickname, i2pAddress)
	}
}

// handleFileOffer обрабатывает предложение файла
func (s *Service) handleFileOffer(packet *pb.Packet, senderPubKey string) {
	offer := &pb.FileOffer{}
	if err := proto.Unmarshal(packet.Payload, offer); err != nil {
		log.Printf("[Messenger] Failed to unmarshal FileOffer: %v", err)
		return
	}

	log.Printf("[Messenger] File offer from %s: %d files", senderPubKey[:min(16, len(senderPubKey))], offer.FileCount)
	if s.fileOfferHandler != nil {
		s.fileOfferHandler(senderPubKey, offer.MessageId, offer.ChatId, offer.Filenames, offer.TotalSize, offer.FileCount)
	}
}

// handleFileResponse обрабатывает ответ на предложение
func (s *Service) handleFileResponse(packet *pb.Packet, senderPubKey string) {
	resp := &pb.FileResponse{}
	if err := proto.Unmarshal(packet.Payload, resp); err != nil {
		log.Printf("[Messenger] Failed to unmarshal FileResponse: %v", err)
		return
	}

	log.Printf("[Messenger] File response from %s: accepted=%v", senderPubKey[:min(16, len(senderPubKey))], resp.Accepted)
	if s.fileResponseHandler != nil {
		s.fileResponseHandler(senderPubKey, resp.MessageId, resp.ChatId, resp.Accepted)
	}
}

// heartbeatLoop отправляет heartbeat всем активным соединениям
func (s *Service) heartbeatLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sendHeartbeatToAll()
		}
	}
}

// sendHeartbeatToAll отправляет heartbeat всем подключенным пирам
func (s *Service) sendHeartbeatToAll() {
	s.connMu.RLock()
	destinations := make([]string, 0, len(s.connections))
	for dest := range s.connections {
		destinations = append(destinations, dest)
	}
	s.connMu.RUnlock()

	for _, dest := range destinations {
		if err := s.SendHeartbeat(dest); err != nil {
			log.Printf("[Messenger] Heartbeat failed for %s...: %v", dest[:min(32, len(dest))], err)
		}
	}

	if len(destinations) > 0 {
		log.Printf("[Messenger] Sent heartbeat to %d peers", len(destinations))
	}
}

// min возвращает минимум из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetProfileUpdateHandler sets the profile update handler
func (s *Service) SetProfileUpdateHandler(h ProfileUpdateHandler) {
	s.profileHandler = h
}

// SetFileOfferHandler sets the file offer handler
func (s *Service) SetFileOfferHandler(h FileOfferHandler) {
	s.fileOfferHandler = h
}

// SetFileResponseHandler sets the file response handler
func (s *Service) SetFileResponseHandler(h FileResponseHandler) {
	s.fileResponseHandler = h
}

// Broadcast sends a packet to all connected peers
func (s *Service) Broadcast(packet *pb.Packet) {
	s.connMu.RLock()
	destinations := make([]string, 0, len(s.connections))
	for dest := range s.connections {
		destinations = append(destinations, dest)
	}
	s.connMu.RUnlock()

	for _, dest := range destinations {
		_ = s.SendMessage(dest, packet)
	}
}

// SetProfileRequestHandler sets the profile request handler
func (s *Service) SetProfileRequestHandler(h ProfileRequestHandler) {
	s.profileRequestHandler = h
}
