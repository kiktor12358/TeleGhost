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
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}/i2pd/libi2pd -I${SRCDIR}/i2pd/libi2pd_client -I${SRCDIR}/i2pd/i18n -I${SRCDIR}/i2pd -I${SRCDIR}/i2pd/libi2pd_wrapper

#cgo windows LDFLAGS: -L${SRCDIR} -Wl,--whole-archive -li2pdclient -li2pd -li2pdlang -Wl,--no-whole-archive -lssl -lcrypto -lz -lboost_filesystem-mt -lboost_program_options-mt -lboost_date_time-mt -lpthread -lws2_32 -lmswsock -liphlpapi -lcrypt32 -ladvapi32 -lshlwapi -luser32 -lgdi32 -lbcrypt -lstdc++ -lm


#include <stdlib.h>

// C wrapper functions for C++ i2pd API
// These are defined in i2pd_wrapper.cpp

extern void i2pd_init(const char* datadir, int sam_enabled, int sam_port, int debug_mode, int tunnel_length, int log_to_file);
extern void i2pd_start();
extern void i2pd_stop();
extern void i2pd_terminate();
extern int i2pd_is_running();
extern const char* i2pd_get_b32_address();
*/
import "C"

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// Router представляет встроенный i2pd роутер
type Router struct {
	dataDir      string
	samPort      int
	samEnabled   bool
	debug        bool
	tunnelLength int
	logToFile    bool
	running      bool
	mu           sync.RWMutex
}

// Config конфигурация роутера
type Config struct {
	// DataDir — директория для данных i2pd
	DataDir string

	// SAMEnabled — включить SAM API
	SAMEnabled bool

	// SAMPort — порт SAM API
	SAMPort int

	// Debug включает подробное логирование i2pd
	Debug bool

	// TunnelLength - длина туннелей (1, 3, 5)
	TunnelLength int

	// LogToFile - писать ли логи в файл
	LogToFile bool
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		DataDir:      ".teleghost/i2pd",
		SAMEnabled:   true,
		SAMPort:      7656,
		TunnelLength: 1,
		LogToFile:    false,
	}
}

// NewRouter создаёт новый встроенный i2pd роутер
func NewRouter(config *Config) *Router {
	if config == nil {
		config = DefaultConfig()
	}
	return &Router{
		dataDir:      config.DataDir,
		samPort:      config.SAMPort,
		samEnabled:   config.SAMEnabled,
		debug:        config.Debug,
		tunnelLength: config.TunnelLength,
		logToFile:    config.LogToFile,
	}
}

// Start запускает i2pd роутер
func (r *Router) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("router already running")
	}

	// Обеспечиваем наличие директории
	if err := os.MkdirAll(r.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// Копируем сертификаты, если их нет или папка пуста
	certsDir := filepath.Join(r.dataDir, "certificates")
	shouldCopy := false
	if _, err := os.Stat(certsDir); os.IsNotExist(err) {
		shouldCopy = true
	} else {
		// Проверяем, не пуста ли папка
		files, _ := os.ReadDir(certsDir)
		if len(files) == 0 {
			shouldCopy = true
		}
	}

	if shouldCopy {
		log.Printf("[i2pd] Copying certificates to %s...", certsDir)
		if err := copyEmbeddedCerts(certsDir); err != nil {
			log.Printf("[i2pd] Warning: failed to copy certificates: %v", err)
		}
	}

	// Не создаём i2pd.conf, полагаемся на аргументы CLI
	// Это избегает конфликтов конфигурации

	// Инициализируем i2pd
	dataDir := C.CString(r.dataDir)
	defer C.free(unsafe.Pointer(dataDir))

	samEnabled := C.int(0)
	if r.samEnabled {
		samEnabled = C.int(1)
	}

	debugMode := C.int(0)
	if r.debug {
		debugMode = C.int(1)
	}

	log.Printf("[i2pd] Initializing with datadir=%s, SAM=%v, port=%d, debug=%v, tunnels=%d, logfile=%v", r.dataDir, r.samEnabled, r.samPort, r.debug, r.tunnelLength, r.logToFile)

	fileLog := C.int(0)
	if r.logToFile {
		fileLog = C.int(1)
	}

	C.i2pd_init(dataDir, samEnabled, C.int(r.samPort), debugMode, C.int(r.tunnelLength), fileLog)
	C.i2pd_start()

	r.running = true
	log.Printf("[i2pd] Router started, waiting for services...")

	// Ждём готовности SAM (блокирующе)
	if err := r.waitForReady(ctx); err != nil {
		r.Stop()
		return err
	}

	return nil
}

// waitForReady ожидает готовности роутера и SAM моста
func (r *Router) waitForReady(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for SAM bridge to be ready")
		case <-ticker.C:
			// Проверяем через C++ обертку
			if C.i2pd_is_running() == 1 {
				// Дополнительно проверяем, слушает ли порт (SAM) и отвечает ли он
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", r.samPort), 200*time.Millisecond)
				if err == nil {
					// Пробуем поздороваться
					conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
					_, err := conn.Write([]byte("HELLO VERSION MIN=3.0 MAX=3.3\n"))
					if err == nil {
						buf := make([]byte, 128)
						n, err := conn.Read(buf)
						if err == nil && n > 0 && strings.Contains(string(buf[:n]), "HELLO REPLY") {
							conn.Close()
							log.Printf("[i2pd] Router and SAM bridge are ready")
							return nil
						}
					}
					conn.Close()
				}
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

	log.Println("[I2PD] Stopping router services...")

	// Create a channel to signal completion
	done := make(chan bool, 1)
	go func() {
		C.i2pd_stop()
		C.i2pd_terminate()
		done <- true
	}()

	// Wait up to 5 seconds for clean stop
	select {
	case <-done:
		log.Println("[I2PD] Router stopped cleanly.")
	case <-time.After(5 * time.Second):
		log.Println("[I2PD] Warning: i2pd stop timed out, continuing shutdown...")
	}

	r.running = false
	return nil
}

// IsReady проверяет, готов ли роутер и SAM мост
func (r *Router) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.running || C.i2pd_is_running() != 1 {
		return false
	}

	// Дополнительная проверка на возможность подключения
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", r.samPort), 50*time.Millisecond)
	if err == nil {
		conn.Close()
		return true
	}
	return false
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

//go:embed i2pd/contrib/certificates
var embeddedCerts embed.FS

// copyEmbeddedCerts extracts embedded certificates to the destination recursively
func copyEmbeddedCerts(dst string) error {
	log.Println("[i2pd] Extracting embedded certificates...")

	return fs.WalkDir(embeddedCerts, "i2pd/contrib/certificates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from the root of embed
		relPath, err := filepath.Rel("i2pd/contrib/certificates", path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0700)
		}

		data, err := embeddedCerts.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, 0644)
	})
}
