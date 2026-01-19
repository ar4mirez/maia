package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		defaultVal  string
		envVal      string
		setEnv      bool
		expected    string
	}{
		{
			name:       "env not set",
			key:        "TEST_KEY_NOT_SET",
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "env is set",
			key:        "TEST_KEY_SET",
			defaultVal: "default",
			envVal:     "from_env",
			setEnv:     true,
			expected:   "from_env",
		},
		{
			name:       "env is empty",
			key:        "TEST_KEY_EMPTY",
			defaultVal: "default",
			envVal:     "",
			setEnv:     true,
			expected:   "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envVal)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvOrDefault(tt.key, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecute(t *testing.T) {
	// Reset the command for testing
	rootCmd.SetArgs([]string{"--help"})
	err := Execute()
	assert.NoError(t, err)
}
