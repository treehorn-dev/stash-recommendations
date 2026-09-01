package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsAndValidatesModelOWeight(t *testing.T) {
	t.Setenv("MODEL_O_WEIGHT", "")
	config, err := Load()
	require.NoError(t, err)
	require.Equal(t, 1.5, config.ModelOWeight)

	t.Setenv("MODEL_O_WEIGHT", "0")
	_, err = Load()
	require.EqualError(t, err, "MODEL_O_WEIGHT must be a positive number")
}
