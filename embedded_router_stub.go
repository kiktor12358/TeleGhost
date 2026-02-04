//go:build !cgo_i2pd
// +build !cgo_i2pd

package main

import (
	"context"
	"log"
)

// initEmbeddedRouter stub for non-CGO build
func (a *App) initEmbeddedRouter(ctx context.Context) error {
	log.Printf("[App] Using external I2P router (embedded router disabled)")
	return nil
}
