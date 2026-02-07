//go:build cgo_i2pd
// +build cgo_i2pd

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"teleghost/internal/network/i2pd"
)

// initEmbeddedRouter initializes and starts the embedded I2P router
func (a *App) initEmbeddedRouter(ctx context.Context) error {
	log.Printf("[App] Initializing embedded i2pd router...")

	i2pdDir := filepath.Join(a.dataDir, "i2pd")

	// Check for debug mode (default to false unless strictly requested/dev)
	// But user asked for a behavior where it writes logs ONLY if debug is needed.
	// Let's check environment variable for now.
	debugMode := false
	// TODO: Add a proper settings switch in UI later
	if os.Getenv("TELEGHOST_DEBUG") == "1" {
		debugMode = true
	}
	// User: "make a debug mode that will write this and nothing else is needed"
	// I will default to FALSE, so it's clean by default. User can set env or I can tell them how to enable.
	// Use TELEGHOST_DEBUG=1 to enable logging.

	cfg := &i2pd.Config{
		DataDir:    i2pdDir,
		SAMEnabled: true,
		SAMPort:    7656,
		Debug:      debugMode,
	}

	router := i2pd.NewRouter(cfg)
	a.embeddedRouter = router // Сохраняем для синхронизации
	a.embeddedStop = router.Stop

	// Запускаем в фоне, чтобы не блокировать startup
	go func() {
		if err := router.Start(ctx); err != nil {
			log.Printf("[App] Embedded router failed to start: %v", err)
		}
	}()

	return nil
}
