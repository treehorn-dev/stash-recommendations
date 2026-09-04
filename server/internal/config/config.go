package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// Config contains the process configuration supplied through the environment.
type Config struct {
	HTTPAddr             string
	DatabaseURL          string
	ModelOWeight         float64
	ModelRefreshInterval time.Duration
	BuildModelOnStart    bool
	RebuildModelOnce     bool
}

// Load reads the service configuration from the environment.
func Load() (Config, error) {
	oWeight := 1.5
	if raw := os.Getenv("MODEL_O_WEIGHT"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return Config{}, fmt.Errorf("MODEL_O_WEIGHT must be a positive number")
		}
		oWeight = parsed
	}
	refreshInterval := 5 * time.Minute
	if raw := os.Getenv("MODEL_REFRESH_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < 0 {
			return Config{}, fmt.Errorf("MODEL_REFRESH_INTERVAL must be a Go duration")
		}
		refreshInterval = parsed
	}
	buildModelOnStart := false
	if raw := os.Getenv("BUILD_MODEL_ON_START"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("BUILD_MODEL_ON_START must be a boolean")
		}
		buildModelOnStart = parsed
	}
	rebuildModelOnce := false
	if raw := os.Getenv("REBUILD_MODEL_ONCE"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("REBUILD_MODEL_ONCE must be a boolean")
		}
		rebuildModelOnce = parsed
	}
	return Config{
		HTTPAddr:             os.Getenv("HTTP_ADDR"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		ModelOWeight:         oWeight,
		ModelRefreshInterval: refreshInterval,
		BuildModelOnStart:    buildModelOnStart,
		RebuildModelOnce:     rebuildModelOnce,
	}, nil
}
