// Package router предоставляет абстракцию I2P роутера через SAM3 API
package router

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/sam3"
)

// Router — интерфейс I2P роутера
type Router interface {
	// Start запускает роутер
	Start(ctx context.Context) error

	// Stop останавливает роутер
	Stop() error

	// GetSAMAddress возвращает адрес SAM Bridge
	GetSAMAddress() string

	// GetDestination возвращает наш I2P Destination
	GetDestination() string

	// IsReady проверяет готовность роутера
	IsReady() bool
}

// Config конфигурация роутера
type Config struct {
	// SAMAddress — адрес SAM bridge
	SAMAddress string

	// DataDir — директория для данных I2P
	DataDir string

	// SessionName — имя SAM сессии
	SessionName string

	// InboundLength — длина входящего туннеля
	InboundLength int

	// OutboundLength — длина исходящего туннеля
	OutboundLength int

	// InboundQuantity — количество входящих туннелей
	InboundQuantity int

	// OutboundQuantity — количество исходящих туннелей
	OutboundQuantity int

	// UseNTCP2Only — использовать только NTCP2 (SSU2 нестабилен)
	UseNTCP2Only bool
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		SAMAddress:       "127.0.0.1:7656",
		DataDir:          ".teleghost/i2p",
		SessionName:      fmt.Sprintf("TeleGhost-%d", time.Now().Unix()),
		InboundLength:    1,
		OutboundLength:   1,
		InboundQuantity:  2,
		OutboundQuantity: 2,
		UseNTCP2Only:     true,
	}
}

// SAMRouter — реализация роутера через SAM API
type SAMRouter struct {
	config      *Config
	sam         *sam3.SAM
	session     *sam3.StreamSession
	keys        i2pkeys.I2PKeys
	destination string
	listener    *sam3.StreamListener
	ready       bool
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewSAMRouter создаёт новый SAM роутер
func NewSAMRouter(config *Config) *SAMRouter {
	if config == nil {
		config = DefaultConfig()
	}
	return &SAMRouter{
		config: config,
	}
}

// Start подключается к SAM и создаёт сессию
func (r *SAMRouter) Start(ctx context.Context) error {
	r.mu.Lock()
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.mu.Unlock() // Разблокируем сразу, чтобы можно было вызвать Stop()

	// Пробуем подключиться к SAM с ретраями
	// Используем r.config.SAMAddress без лока (он не меняется)
	var samConn *sam3.SAM
	var err error

	log.Printf("[SAMRouter] Connecting to SAM at %s...", r.config.SAMAddress)
	for i := 0; i < 30; i++ {
		// Проверяем отмену перед попыткой
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}

		samConn, err = sam3.NewSAM(r.config.SAMAddress)
		if err == nil {
			break
		}
		log.Printf("[SAMRouter] Attempt %d failed: %v. Retrying in 3s...", i+1, err)
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	if err != nil {
		return fmt.Errorf("failed to connect to SAM at %s: %w", r.config.SAMAddress, err)
	}

	// Сохраняем samConn сразу, чтобы Stop мог его закрыть
	r.mu.Lock()
	if r.ctx.Err() != nil {
		samConn.Close()
		r.mu.Unlock()
		return r.ctx.Err()
	}
	r.sam = samConn
	r.mu.Unlock()

	log.Printf("[SAMRouter] Connected to SAM bridge")

	// Если ключи не заданы, генерируем новые
	// Используем копию ключей или проверяем под блокировкой, если нужно.
	// Но r.keys доступны только нам пока.
	r.mu.RLock()
	hasKeys := r.keys.String() != ""
	r.mu.RUnlock()

	if !hasKeys {
		log.Println("[SAMRouter] Generating new I2P keys... (this may take a while)")
		keys, errKeys := samConn.NewKeys()
		if errKeys != nil {
			// Проверяем, не вызвана ли ошибка закрытием сокета
			r.mu.Lock()
			defer r.mu.Unlock()
			// Если контекст отменен, возвращаем ошибку контекста
			if r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			return fmt.Errorf("failed to generate I2P keys: %w", errKeys)
		}

		r.mu.Lock()
		r.keys = keys
		r.destination = keys.Addr().Base64()
		r.mu.Unlock()

		log.Println("[SAMRouter] New keys generated")
	} else {
		log.Println("[SAMRouter] Using existing I2P keys")
	}

	// Еще раз проверяем перед созданием сессии
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}

	// Формируем опции для сессии
	// Формируем опции для сессии
	opts := []string{
		fmt.Sprintf("inbound.length=%d", r.config.InboundLength),
		fmt.Sprintf("outbound.length=%d", r.config.OutboundLength),
		fmt.Sprintf("inbound.quantity=%d", r.config.InboundQuantity),
		fmt.Sprintf("outbound.quantity=%d", r.config.OutboundQuantity),
		"inbound.allowZeroHop=true",
		"outbound.allowZeroHop=true",
	}

	// Создаём Streaming сессию
	log.Printf("[SAMRouter] Creating stream session '%s'...", r.config.SessionName)

	// Получаем текущие ключи для сессии
	r.mu.RLock()
	currentKeys := r.keys
	r.mu.RUnlock()

	session, errSession := samConn.NewStreamSession(r.config.SessionName, currentKeys, opts)
	if errSession != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		return fmt.Errorf("failed to create SAM session: %w", errSession)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Финальная проверка
	if r.ctx.Err() != nil {
		session.Close()
		return r.ctx.Err()
	}

	r.session = session
	r.ready = true

	log.Printf("[SAMRouter] Session established")
	return nil
}

// SetKeys устанавливает I2P ключи
func (r *SAMRouter) SetKeys(keys i2pkeys.I2PKeys) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = keys
	r.destination = keys.Addr().Base64()
}

// Stop останавливает роутер
func (r *SAMRouter) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Printf("[SAMRouter] Stopping...")

	if r.cancel != nil {
		r.cancel()
	}

	r.ready = false

	if r.listener != nil {
		_ = r.listener.Close()
		r.listener = nil
	}

	// Закрываем сессию (это закроет и connection-ы от нее)
	if r.session != nil {
		_ = r.session.Close()
		r.session = nil
	}

	// Закрываем SAM соединение (это прервет создание ключей или сессии)
	if r.sam != nil {
		_ = r.sam.Close()
		r.sam = nil
	}

	log.Printf("[SAMRouter] Stopped")
	return nil
}

// GetSAMAddress возвращает адрес SAM Bridge
func (r *SAMRouter) GetSAMAddress() string {
	return r.config.SAMAddress
}

// GetDestination возвращает наш I2P Destination (base64)
func (r *SAMRouter) GetDestination() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.destination
}

// IsReady проверяет готовность
func (r *SAMRouter) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

// GetSession возвращает SAM сессию
func (r *SAMRouter) GetSession() *sam3.StreamSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.session
}

// GetKeys возвращает I2P ключи
func (r *SAMRouter) GetKeys() i2pkeys.I2PKeys {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.keys
}

// Dial устанавливает соединение с удалённым пиром
func (r *SAMRouter) Dial(destination string) (net.Conn, error) {
	r.mu.RLock()
	session := r.session
	samConn := r.sam
	r.mu.RUnlock()

	if session == nil || samConn == nil {
		return nil, fmt.Errorf("router not started")
	}

	// Parse destination address
	// Point 6: Ensure SAM connection is still alive, reconnect if needed
	var addr i2pkeys.I2PAddr
	var err error

	for i := 0; i < 2; i++ {
		addr, err = samConn.Lookup(destination)
		if err == nil {
			break
		}

		// If lookup fails, maybe SAM connection is stale
		if i == 0 {
			log.Printf("[SAMRouter] Lookup failed: %v. Attempting to refresh SAM connection...", err)
			// We can't easily recreate the whole session here without locking everything,
			// but we can try to re-create the SAM bridge connection if possible.
			// Actually, if Lookup fails, it's likely the sam socket is dead.
			// Try to recreate sam3.SAM
			newSam, samErr := sam3.NewSAM(r.config.SAMAddress)
			if samErr == nil {
				r.mu.Lock()
				r.sam.Close()
				r.sam = newSam
				samConn = newSam
				r.mu.Unlock()
				continue
			}
		}

		// Try direct parsing as fallback
		addr, err = i2pkeys.NewI2PAddrFromString(destination)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("invalid destination: %w", err)
	}

	conn, err := session.DialI2P(addr)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	return conn, nil
}

// Listen создаёт listener для входящих соединений
func (r *SAMRouter) Listen() (*sam3.StreamListener, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session == nil {
		return nil, fmt.Errorf("router not started")
	}

	if r.listener == nil {
		listener, err := r.session.Listen()
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		r.listener = listener
	}

	return r.listener, nil
}

// Accept ожидает входящее соединение
func (r *SAMRouter) Accept() (net.Conn, error) {
	listener, err := r.Listen()
	if err != nil {
		return nil, err
	}

	return listener.Accept()
}
