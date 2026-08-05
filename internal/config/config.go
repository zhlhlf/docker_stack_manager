package config

import (
	"flag"
	"os"
)

// Config holds runtime configuration.
type Config struct {
	ListenAddr string
	DBPath     string
}

// Load parses flags and environment variables.
func Load() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.ListenAddr, "addr", getEnv("LISTEN_ADDR", ":8080"), "HTTP listen address")
	flag.StringVar(&cfg.DBPath, "db", getEnv("DB_PATH", "data.json"), "JSON database path")
	flag.Parse()
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}