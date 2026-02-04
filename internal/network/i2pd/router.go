//go:build cgo_i2pd
// +build cgo_i2pd

// Package i2pd provides CGO bindings to the i2pd C++ router library.
// This allows embedding a full I2P router directly into Go applications.
//
// Build Requirements (Debian/Ubuntu):
//
//	sudo apt install build-essential cmake libboost-all-dev libssl-dev zlib1g-dev
//
// You must build libi2pd first:
//
//	git clone https://github.com/PurpleI2P/i2pd.git
//	cd i2pd && make libi2pd.a
//
// To build with embedded i2pd:
//
//	go build -tags cgo_i2pd ./...
package i2pd

/*
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}/i2pd/libi2pd -I${SRCDIR}/i2pd/i18n -I${SRCDIR}/i2pd
#cgo LDFLAGS: -L${SRCDIR}/i2pd -li2pd -lssl -lcrypto -lz -lboost_system -lboost_filesystem -lboost_program_options -lboost_date_time -lpthread -lstdc++

#include <stdlib.h>

// C wrapper functions for C++ i2pd API
// These are defined in i2pd_wrapper.cpp

extern void i2pd_init(const char* datadir, int sam_enabled, int sam_port);
extern void i2pd_start();
extern void i2pd_stop();
extern void i2pd_terminate();
extern int i2pd_is_running();
extern const char* i2pd_get_b32_address();
*/
import "C"

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"
)

// Router представляет встроенный i2pd роутер
type Router struct {
	dataDir    string
	samPort    int
	samEnabled bool
	running    bool
	mu         sync.RWMutex
}

// Config конфигурация роутера
type Config struct {
	// DataDir — директория для данных i2pd
	DataDir string

	// SAMEnabled — включить SAM API
	SAMEnabled bool

	// SAMPort — порт SAM API
	SAMPort int
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		DataDir:    ".teleghost/i2pd",
		SAMEnabled: true,
		SAMPort:    7656,
	}
}

// NewRouter создаёт новый встроенный i2pd роутер
func NewRouter(config *Config) *Router {
	if config == nil {
		config = DefaultConfig()
	}
	return &Router{
		dataDir:    config.DataDir,
		samPort:    config.SAMPort,
		samEnabled: config.SAMEnabled,
	}
}

// Start запускает i2pd роутер
func (r *Router) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("router already running")
	}

	// Инициализируем i2pd
	dataDir := C.CString(r.dataDir)
	defer C.free(unsafe.Pointer(dataDir))

	samEnabled := C.int(0)
	if r.samEnabled {
		samEnabled = C.int(1)
	}

	log.Printf("[i2pd] Initializing with datadir=%s, SAM=%v, port=%d", r.dataDir, r.samEnabled, r.samPort)

	C.i2pd_init(dataDir, samEnabled, C.int(r.samPort))
	C.i2pd_start()

	r.running = true
	log.Printf("[i2pd] Router started")

	// Ждём готовности SAM
	go r.waitForReady(ctx)

	return nil
}

// waitForReady ожидает готовности роутера
func (r *Router) waitForReady(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(120 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			log.Printf("[i2pd] Timeout waiting for router to be ready")
			return
		case <-ticker.C:
			if C.i2pd_is_running() == 1 {
				log.Printf("[i2pd] Router is ready")
				return
			}
		}
	}
}

// Stop останавливает i2pd роутер
func (r *Router) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	log.Printf("[i2pd] Stopping router...")
	C.i2pd_stop()
	C.i2pd_terminate()

	r.running = false
	log.Printf("[i2pd] Router stopped")

	return nil
}

// IsRunning проверяет работает ли роутер
func (r *Router) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running && C.i2pd_is_running() == 1
}

// GetB32Address возвращает B32 адрес роутера
func (r *Router) GetB32Address() string {
	addr := C.i2pd_get_b32_address()
	if addr == nil {
		return ""
	}
	return C.GoString(addr)
}

// GetSAMAddress возвращает адрес SAM API
func (r *Router) GetSAMAddress() string {
	return fmt.Sprintf("127.0.0.1:%d", r.samPort)
}
