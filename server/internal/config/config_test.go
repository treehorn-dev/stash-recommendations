package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsAndValidatesModelOWeight(t *testing.T) {
	t.Setenv("MODEL_O_WEIGHT", "")
	config, err := Load()
	require.NoError(t, err)
	require.Equal(t, 1.5, config.ModelOWeight)

	for _, value := range []string{"0", "NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MODEL_O_WEIGHT", value)
			_, err = Load()
			require.EqualError(t, err, "MODEL_O_WEIGHT must be a positive number")
		})
	}
}

func TestLoadDefaultsAndValidatesModelRefreshInterval(t *testing.T) {
	t.Setenv("MODEL_REFRESH_INTERVAL", "")
	config, err := Load()
	require.NoError(t, err)
	require.Equal(t, 5*time.Minute, config.ModelRefreshInterval)

	t.Setenv("MODEL_REFRESH_INTERVAL", "30s")
	config, err = Load()
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, config.ModelRefreshInterval)

	t.Setenv("MODEL_REFRESH_INTERVAL", "not-a-duration")
	_, err = Load()
	require.EqualError(t, err, "MODEL_REFRESH_INTERVAL must be a Go duration")
}

func TestLoadEnablesOneShotModelRebuild(t *testing.T) {
	t.Setenv("REBUILD_MODEL_ONCE", "true")
	config, err := Load()
	require.NoError(t, err)
	require.True(t, config.RebuildModelOnce)
}
