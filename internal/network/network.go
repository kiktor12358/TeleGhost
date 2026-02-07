// Package network предоставляет абстракции для работы с I2P сетью
package network

import (
	"context"
	"io"
)

// Connection представляет соединение с пиром
type Connection interface {
	io.ReadWriteCloser

	// RemoteAddr возвращает I2P адрес удалённого пира
	RemoteAddr() string

	// LocalAddr возвращает наш I2P адрес
	LocalAddr() string
}

// NetworkRouter — интерфейс для работы с I2P сетью
// Это позволяет легко подменить реализацию (go-i2p или i2pd)
type NetworkRouter interface {
	// Start запускает роутер
	Start(ctx context.Context) error

	// Stop останавливает роутер
	Stop() error

	// GetAddress возвращает наш I2P destination адрес
	GetAddress() (string, error)

	// Connect устанавливает соединение с пиром по его I2P адресу
	Connect(ctx context.Context, destination string) (Connection, error)

	// Accept ожидает входящее соединение
	Accept(ctx context.Context) (Connection, error)

	// IsReady проверяет готовность роутера к работе
	IsReady() bool
}

// RouterConfig содержит конфигурацию роутера
type RouterConfig struct {
	// DataDir — директория для хранения данных I2P
	DataDir string

	// SAMAddress — адрес SAM bridge (если используем внешний i2pd)
	SAMAddress string

	// UseSAM — использовать SAM API вместо встроенного роутера
	UseSAM bool

	// InboundLength — длина входящего туннеля
	InboundLength int

	// OutboundLength — длина исходящего туннеля
	OutboundLength int

	// InboundQuantity — количество входящих туннелей
	InboundQuantity int

	// OutboundQuantity — количество исходящих туннелей
	OutboundQuantity int
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *RouterConfig {
	return &RouterConfig{
		DataDir:          ".teleghost/i2p",
		SAMAddress:       "127.0.0.1:7656",
		UseSAM:           true,
		InboundLength:    3,
		OutboundLength:   3,
		InboundQuantity:  2,
		OutboundQuantity: 2,
	}
}
