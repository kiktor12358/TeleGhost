// Package messenger реализует сервис обмена сообщениями через I2P
package messenger

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
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

	// MaxPacketSize максимальный размер пакета (64KB)
	MaxPacketSize = 64 * 1024

	// HeartbeatInterval интервал отправки heartbeat
	HeartbeatInterval = 60 * time.Second

	// ConnectionTimeout таймаут соединения (для I2P нужны большие значения)
	ConnectionTimeout = 3 * time.Minute

	// ReadTimeout таймаут чтения (I2P медленный, особенно при первом подключении)
	ReadTimeout = 5 * time.Minute
)

// MessageHandler обработчик входящих сообщений
type MessageHandler func(msg *core.Message, senderPubKey, senderAddr string)

// ContactRequestHandler обработчик входящих запросов "дружбы" (handshake)
type ContactRequestHandler func(pubKey, nickname, i2pAddress string)

// ProfileUpdateHandler handler
type ProfileUpdateHandler func(pubKey, nickname, bio string, avatar []byte)

// Service — мессенджер сервис
type Service struct {
	router         *router.SAMRouter
	identity       *identity.Keys
	handler        MessageHandler
	contactHandler ContactRequestHandler
	profileHandler ProfileUpdateHandler
	connections    map[string]net.Conn // destination -> connection
	connMu         sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	started        bool
	mu             sync.Mutex
	myNickname     string // Для отправки в handshake
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

	log.Printf("[Messenger] Started. My destination: %s...", s.router.GetDestination()[:32])

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
		conn.Close()
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
	conn, err := s.getOrCreateConnection(destination)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Сериализуем пакет
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	if len(data) > MaxPacketSize {
		return fmt.Errorf("packet too large: %d > %d", len(data), MaxPacketSize)
	}

	// Отправляем: 4 байта размер + данные
	if err := s.writePacket(conn, data); err != nil {
		// При ошибке удаляем соединение из пула
		s.removeConnection(destination)
		return fmt.Errorf("send failed: %w", err)
	}

	return nil
}

// SendTextMessage создаёт и отправляет текстовое сообщение
func (s *Service) SendTextMessage(destination, chatID, content string) error {
	now := time.Now().UnixMilli()

	// Создаём TextMessage
	textMsg := &pb.TextMessage{
		ChatId:    chatID,
		Content:   content,
		Timestamp: now,
		MessageId: fmt.Sprintf("%d-%s", now, s.identity.UserID[:8]),
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

	log.Printf("[Messenger] Sending handshake to %s...", destination[:32])
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

	// Создаём новое соединение
	s.connMu.Lock()
	defer s.connMu.Unlock()

	// Double-check
	if conn, exists = s.connections[destination]; exists {
		return conn, nil
	}

	// Dial с таймаутом
	newConn, err := s.router.Dial(destination)
	if err != nil {
		return nil, err
	}

	s.connections[destination] = newConn
	log.Printf("[Messenger] Connected to %s...", destination[:32])

	return newConn, nil
}

// removeConnection удаляет соединение из пула
func (s *Service) removeConnection(destination string) {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if conn, exists := s.connections[destination]; exists {
		conn.Close()
		delete(s.connections, destination)
	}
}

// writePacket пишет пакет в соединение (length-prefixed)
func (s *Service) writePacket(conn net.Conn, data []byte) error {
	// Устанавливаем deadline
	conn.SetWriteDeadline(time.Now().Add(ConnectionTimeout))

	// Пишем размер (4 байта, big endian)
	sizeBuf := make([]byte, 4)
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
	conn.SetReadDeadline(time.Now().Add(ReadTimeout))

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
			if err != nil && (err.Error() == "use of closed network connection" || err.Error() == "listener closed") {
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
		s.handleProfileUpdate(packet, senderPubKey)

	case pb.PacketType_HANDSHAKE:
		s.handleHandshake(packet, senderPubKey)

	default:
		log.Printf("[Messenger] Unknown packet type: %v", packet.Type)
	}
}

// handleTextMessage обрабатывает текстовое сообщение
func (s *Service) handleTextMessage(packet *pb.Packet, senderPubKey, remoteAddr string) {
	textMsg := &pb.TextMessage{}
	if err := proto.Unmarshal(packet.Payload, textMsg); err != nil {
		log.Printf("[Messenger] Failed to unmarshal TextMessage: %v", err)
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

	log.Printf("[Messenger] Received message: %s...", textMsg.Content[:min(20, len(textMsg.Content))])

	// Вызываем callback для обработки
	if s.handler != nil {
		s.handler(msg, senderPubKey, remoteAddr)
	}
}

// handleProfileUpdate обрабатывает обновление профиля
func (s *Service) handleProfileUpdate(packet *pb.Packet, senderPubKey string) {
	profileUpdate := &pb.ProfileUpdate{}
	if err := proto.Unmarshal(packet.Payload, profileUpdate); err != nil {
		log.Printf("[Messenger] Failed to unmarshal ProfileUpdate: %v", err)
		return
	}

	log.Printf("[Messenger] Profile update from %s: %s", senderPubKey[:16], profileUpdate.Nickname)
	if s.profileHandler != nil {
		s.profileHandler(senderPubKey, profileUpdate.Nickname, profileUpdate.Bio, profileUpdate.Avatar)
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

	log.Printf("[Messenger] Handshake from %s (nickname: %s)", senderPubKey[:16], nickname)

	// Вызываем callback для создания контакта
	if s.contactHandler != nil {
		s.contactHandler(senderPubKey, nickname, i2pAddress)
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
			log.Printf("[Messenger] Heartbeat failed for %s...: %v", dest[:32], err)
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
