//go:build cgo_i2pd
// +build cgo_i2pd

package main

import (
	"context"
	"log"
	"path/filepath"
	"teleghost/internal/network/i2pd"
)

// initEmbeddedRouter initializes and starts the embedded I2P router
func (a *App) initEmbeddedRouter(ctx context.Context) error {
	log.Printf("[App] Initializing embedded i2pd router...")

	i2pdDir := filepath.Join(a.dataDir, "i2pd")
	cfg := &i2pd.Config{
		DataDir:    i2pdDir,
		SAMEnabled: true,
		SAMPort:    7656,
	}

	router := i2pd.NewRouter(cfg)

	if err := router.Start(ctx); err != nil {
		return err
	}

	a.embeddedStop = router.Stop
	return nil
}
