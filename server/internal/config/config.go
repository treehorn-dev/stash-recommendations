package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config contains the process configuration supplied through the environment.
type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	ModelOWeight      float64
	BuildModelOnStart bool
}

// Load reads the service configuration from the environment.
func Load() (Config, error) {
	oWeight := 1.5
	if raw := os.Getenv("MODEL_O_WEIGHT"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("MODEL_O_WEIGHT must be a positive number")
		}
		oWeight = parsed
	}
	buildModelOnStart := false
	if raw := os.Getenv("BUILD_MODEL_ON_START"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("BUILD_MODEL_ON_START must be a boolean")
		}
		buildModelOnStart = parsed
	}
	return Config{
		HTTPAddr:          os.Getenv("HTTP_ADDR"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		ModelOWeight:      oWeight,
		BuildModelOnStart: buildModelOnStart,
	}, nil
}
