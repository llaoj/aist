// Package server contains the AIST server (PocketBase-based) business logic.
package server

import (
	"os"

	"github.com/llaoj/aist/internal/util"
)

// Config holds server runtime settings sourced from the environment.
type Config struct {
	// Addr is the listen address, e.g. ":8090".
	Addr string
}

// LoadConfig reads server settings from the environment, applying defaults.
func LoadConfig() Config {
	return Config{
		Addr: util.FirstNonEmpty(os.Getenv("AIST_SERVER_ADDR"), ":8090"),
	}
}
