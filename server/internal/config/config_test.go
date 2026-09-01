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

	for _, value := range []string{"0", "NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MODEL_O_WEIGHT", value)
			_, err = Load()
			require.EqualError(t, err, "MODEL_O_WEIGHT must be a positive number")
		})
	}
}
