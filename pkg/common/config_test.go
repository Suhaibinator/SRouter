package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteOverrides_HasTransaction(t *testing.T) {
	tests := []struct {
		name     string
		override RouteOverrides
		want     bool
	}{
		{
			name:     "no transaction config",
			override: RouteOverrides{},
			want:     false,
		},
		{
			name: "with transaction config",
			override: RouteOverrides{
				Transaction: &TransactionConfig{
					Enabled: true,
				},
			},
			want: true,
		},
		{
			name: "with disabled transaction config",
			override: RouteOverrides{
				Transaction: &TransactionConfig{
					Enabled: false,
				},
			},
			want: true, // Still has config, even if disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.override.HasTransaction())
		})
	}
}

func TestTransactionConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		tc := &TransactionConfig{}
		assert.False(t, tc.Enabled)
		assert.Nil(t, tc.Options)
	})

	t.Run("with options", func(t *testing.T) {
		tc := &TransactionConfig{
			Enabled: true,
			Options: map[string]any{
				"isolation": "read-committed",
				"timeout":   30,
			},
		}
		assert.True(t, tc.Enabled)
		assert.Equal(t, "read-committed", tc.Options["isolation"])
		assert.Equal(t, 30, tc.Options["timeout"])
	})
}