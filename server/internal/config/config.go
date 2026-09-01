package config

import "os"

// Config contains the process configuration supplied through the environment.
type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load reads the service configuration from the environment.
func Load() (Config, error) {
	return Config{
		HTTPAddr:    os.Getenv("HTTP_ADDR"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}, nil
}
